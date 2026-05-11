package integration_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps/harness"
	"github.com/impire-io/imps/testutil/natstest"
)

func concurrencySpec(reasoning harness.ReasoningFn) harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "concurrency",
		Version: "0.1.0",
		Channels: []harness.ChannelSpec{{
			Name:   "inbound",
			Source: harness.SubjectSource{Subject: "messages.in"},
			Decode: func(msg harness.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(decoded any) (harness.Entity, error) {
				return harness.Entity(decoded.(string)), nil
			},
		}},
		Awareness: func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Wake(decoded, e)
		},
		Reasoning: reasoning,
		Actions:   []string{"actions.out"},
	}
}

func freshImp(t *testing.T, spec harness.ImpSpec, opts ...harness.Option) (*harness.Imp, *nats.Conn, func()) {
	t.Helper()
	srv := natstest.New(t)
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })
	defaults := []harness.Option{harness.WithSubjectPrefix("test"), harness.WithDrainWindow(2 * time.Second)}
	imp, err := harness.NewImp(spec, nc, append(defaults, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()
	waitReady(t, imp)
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	return imp, nc, func() {
		cancel()
		<-runErr
	}
}

func TestConcurrentReasoningDistinctEntities(t *testing.T) {
	bothInside := make(chan struct{})
	release := make(chan struct{})

	var inside atomic.Int32
	reasoning := func(ctx context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error {
		if inside.Add(1) == 2 {
			close(bothInside)
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}

	imp, nc, cleanup := freshImp(t, concurrencySpec(reasoning))
	defer cleanup()

	if err := nc.Publish("test.messages.in", []byte("E1")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("test.messages.in", []byte("E2")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-bothInside:
	case <-time.After(2 * time.Second):
		t.Fatalf("two reasoning invocations never overlapped (inflight=%d)", imp.Metrics().InflightReasoning)
	}

	if got := imp.Metrics().InflightReasoning; got != 2 {
		t.Fatalf("expected InflightReasoning=2, got %d", got)
	}

	close(release)
}

func TestAwarenessNotBlockedByHeldReasoning(t *testing.T) {
	release := make(chan struct{})
	var awarenessLatency atomic.Int64

	reasoning := func(ctx context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}

	spec := concurrencySpec(reasoning)
	spec.Awareness = func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
		// Record awareness latency for correlation; the test asserts on
		// the dispatch round-trip below.
		awarenessLatency.Store(time.Now().UnixNano())
		return harness.Wake(decoded, e)
	}

	_, nc, cleanup := freshImp(t, spec)
	defer cleanup()

	// First message: trips reasoning that will block.
	if err := nc.Publish("test.messages.in", []byte("blocker")); err != nil {
		t.Fatal(err)
	}

	// Wait briefly for the first awareness to fire.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if awarenessLatency.Load() > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Now publish a second message and measure the time until the second
	// awareness runs. A held reasoning must NOT block this.
	awarenessLatency.Store(0)
	publishTime := time.Now()
	if err := nc.Publish("test.messages.in", []byte("after")); err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if awarenessLatency.Load() > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	latency := time.Since(publishTime)
	if awarenessLatency.Load() == 0 {
		t.Fatalf("second awareness never fired (reasoning blocked dispatch)")
	}
	if latency > 200*time.Millisecond {
		t.Fatalf("awareness latency too high under reasoning load: %v", latency)
	}

	close(release)
}

func TestShutdownDrainWindow(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	const drain = 200 * time.Millisecond
	reasoningStart := make(chan struct{}, 4)
	reasoning := func(ctx context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error {
		reasoningStart <- struct{}{}
		// Block forever; only ctx-cancel can free us.
		<-ctx.Done()
		return nil
	}

	spec := concurrencySpec(reasoning)
	imp, err := harness.NewImp(spec, nc, harness.WithSubjectPrefix("test"), harness.WithDrainWindow(drain))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()
	waitReady(t, imp)
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("test.messages.in", []byte("E1")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("test.messages.in", []byte("E2")); err != nil {
		t.Fatal(err)
	}

	// Wait for both reasonings to start.
	for i := 0; i < 2; i++ {
		select {
		case <-reasoningStart:
		case <-time.After(2 * time.Second):
			t.Fatalf("reasoning %d never started", i+1)
		}
	}

	// Now shut down: drain window is 200 ms; reasoning observes ctx-cancel
	// and returns; shutdown returns within drain + ε.
	t0 := time.Now()
	cancel()
	select {
	case <-runErr:
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within 2s of cancel")
	}
	elapsed := time.Since(t0)
	// Bound: drain window + 500 ms grace for goroutine scheduling. We
	// expect well below this since reasoning cooperatively returns on
	// ctx-cancel.
	if elapsed > drain+500*time.Millisecond {
		t.Fatalf("shutdown took %v, expected < %v", elapsed, drain+500*time.Millisecond)
	}
}

func TestReasoningPanicIsolation(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	publishedAction := make(chan []byte, 4)
	if _, err := nc.Subscribe("test.actions.out", func(m *nats.Msg) { publishedAction <- m.Data }); err != nil {
		t.Fatal(err)
	}

	reasoning := func(ctx context.Context, _ any, e harness.Entity, r harness.ReasoningContext) error {
		switch e {
		case "panic":
			panic("oh no")
		default:
			return r.Publish(ctx, "actions.out", []byte(string(e)))
		}
	}

	spec := concurrencySpec(reasoning)
	imp, err := harness.NewImp(spec, nc, harness.WithSubjectPrefix("test"), harness.WithDrainWindow(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()
	waitReady(t, imp)
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("test.messages.in", []byte("panic")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("test.messages.in", []byte("ok-1")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("test.messages.in", []byte("ok-2")); err != nil {
		t.Fatal(err)
	}

	// Both ok messages must produce actions despite the sibling panic.
	got := map[string]bool{}
	deadline := time.Now().Add(2 * time.Second)
	for len(got) < 2 && time.Now().Before(deadline) {
		select {
		case data := <-publishedAction:
			got[string(data)] = true
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !got["ok-1"] || !got["ok-2"] {
		t.Fatalf("missing actions: %v (metrics=%+v)", got, imp.Metrics())
	}

	// Allow inflight gauge to settle.
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if imp.Metrics().InflightReasoning == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	m := imp.Metrics()
	if m.ReasoningPanics != 1 {
		t.Fatalf("expected ReasoningPanics=1, got %d", m.ReasoningPanics)
	}
	if m.InflightReasoning != 0 {
		t.Fatalf("expected InflightReasoning=0 after settle, got %d", m.InflightReasoning)
	}
}
