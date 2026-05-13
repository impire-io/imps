package integration_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps"
	"github.com/impire-io/imps/testutil/natstest"
)

func concurrencySpec(thinking imps.ThinkingFn) imps.ImpSpec {
	return imps.ImpSpec{
		Name:    "concurrency",
		Version: "0.1.0",
		Channels: []imps.ChannelSpec{{
			Name:   "inbound",
			Source: imps.SubjectSource{Subject: "messages.in"},
			Decode: func(msg imps.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(decoded any) (imps.Entity, error) {
				return imps.Entity(decoded.(string)), nil
			},
		}},
		Awareness: func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			return imps.Think(decoded, e)
		},
		Thinking: thinking,
	}
}

func freshImp(t *testing.T, spec imps.ImpSpec, opts ...imps.Option) (*imps.Imp, *nats.Conn, func()) {
	t.Helper()
	srv := natstest.New(t)
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })
	defaults := []imps.Option{imps.WithDrainWindow(2 * time.Second)}
	imp, err := imps.NewImp(spec, nc, append(defaults, opts...)...)
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

func TestConcurrentThinkingDistinctEntities(t *testing.T) {
	bothInside := make(chan struct{})
	release := make(chan struct{})

	var inside atomic.Int32
	thinking := func(ctx context.Context, _ any, _ imps.Entity, _ imps.ThinkingContext) error {
		if inside.Add(1) == 2 {
			close(bothInside)
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}

	imp, nc, cleanup := freshImp(t, concurrencySpec(thinking))
	defer cleanup()

	if err := nc.Publish("messages.in", []byte("E1")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("messages.in", []byte("E2")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-bothInside:
	case <-time.After(2 * time.Second):
		t.Fatalf("two thinking invocations never overlapped (inflight=%d)", imp.Metrics().InflightThinking)
	}

	if got := imp.Metrics().InflightThinking; got != 2 {
		t.Fatalf("expected InflightThinking=2, got %d", got)
	}

	close(release)
}

func TestAwarenessNotBlockedByHeldThinking(t *testing.T) {
	release := make(chan struct{})
	var awarenessLatency atomic.Int64

	thinking := func(ctx context.Context, _ any, _ imps.Entity, _ imps.ThinkingContext) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}

	spec := concurrencySpec(thinking)
	spec.Awareness = func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
		// Record awareness latency for correlation; the test asserts on
		// the dispatch round-trip below.
		awarenessLatency.Store(time.Now().UnixNano())
		return imps.Think(decoded, e)
	}

	_, nc, cleanup := freshImp(t, spec)
	defer cleanup()

	// First message: trips thinking that will block.
	if err := nc.Publish("messages.in", []byte("blocker")); err != nil {
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
	// awareness runs. A held thinking must NOT block this.
	awarenessLatency.Store(0)
	publishTime := time.Now()
	if err := nc.Publish("messages.in", []byte("after")); err != nil {
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
		t.Fatalf("second awareness never fired (thinking blocked dispatch)")
	}
	if latency > 200*time.Millisecond {
		t.Fatalf("awareness latency too high under thinking load: %v", latency)
	}

	close(release)
}

func TestShutdownDrainWindow(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	const drain = 200 * time.Millisecond
	thinkingStart := make(chan struct{}, 4)
	thinking := func(ctx context.Context, _ any, _ imps.Entity, _ imps.ThinkingContext) error {
		thinkingStart <- struct{}{}
		// Block forever; only ctx-cancel can free us.
		<-ctx.Done()
		return nil
	}

	spec := concurrencySpec(thinking)
	imp, err := imps.NewImp(spec, nc, imps.WithDrainWindow(drain))
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

	if err := nc.Publish("messages.in", []byte("E1")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("messages.in", []byte("E2")); err != nil {
		t.Fatal(err)
	}

	// Wait for both thinkings to start.
	for i := 0; i < 2; i++ {
		select {
		case <-thinkingStart:
		case <-time.After(2 * time.Second):
			t.Fatalf("thinking %d never started", i+1)
		}
	}

	// Now shut down: drain window is 200 ms; thinking observes ctx-cancel
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
	// expect well below this since thinking cooperatively returns on
	// ctx-cancel.
	if elapsed > drain+500*time.Millisecond {
		t.Fatalf("shutdown took %v, expected < %v", elapsed, drain+500*time.Millisecond)
	}
}

func TestThinkingPanicIsolation(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	publishedAction := make(chan []byte, 4)
	if _, err := nc.Subscribe("actions.out", func(m *nats.Msg) { publishedAction <- m.Data }); err != nil {
		t.Fatal(err)
	}

	thinking := func(ctx context.Context, _ any, e imps.Entity, r imps.ThinkingContext) error {
		switch e {
		case "panic":
			panic("oh no")
		default:
			return r.Publish(ctx, "actions.out", []byte(string(e)))
		}
	}

	spec := concurrencySpec(thinking)
	imp, err := imps.NewImp(spec, nc, imps.WithDrainWindow(2*time.Second))
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

	if err := nc.Publish("messages.in", []byte("panic")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("messages.in", []byte("ok-1")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("messages.in", []byte("ok-2")); err != nil {
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
		if imp.Metrics().InflightThinking == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	m := imp.Metrics()
	if m.ThinkingPanics != 1 {
		t.Fatalf("expected ThinkingPanics=1, got %d", m.ThinkingPanics)
	}
	if m.InflightThinking != 0 {
		t.Fatalf("expected InflightThinking=0 after settle, got %d", m.InflightThinking)
	}
}
