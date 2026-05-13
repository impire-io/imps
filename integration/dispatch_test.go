package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps"
	"github.com/impire-io/imps/testutil/natstest"
)

// echoSpec returns a minimal Think-always imp that publishes the decoded
// payload back out on actions.out. The awareness fn passed in lets each
// test customize behavior while sharing the rest of the spec.
func echoSpec(awareness imps.AwarenessFn) imps.ImpSpec {
	return imps.ImpSpec{
		Name:    "echo",
		Version: "0.1.0",
		Channels: []imps.ChannelSpec{{
			Name:   "inbound",
			Source: imps.SubjectSource{Subject: "messages.in"},
			Decode: func(msg imps.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(decoded any) (imps.Entity, error) {
				return imps.Entity("singleton"), nil
			},
		}},
		Awareness: awareness,
		Reasoning: func(ctx context.Context, reason any, _ imps.Entity, r imps.ReasoningContext) error {
			payload := []byte(reason.(string))
			return r.Publish(ctx, "actions.out", payload)
		},
	}
}

func startImp(t *testing.T, spec imps.ImpSpec, opts ...imps.Option) (*imps.Imp, *nats.Conn, func()) {
	t.Helper()
	srv := natstest.New(t)
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatalf("nats.Connect: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	imp, err := imps.NewImp(spec, nc, opts...)
	if err != nil {
		t.Fatalf("NewImp: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()

	// Allow Run to install runtime state and subscriptions.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if imp.Ready() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("nc.Flush: %v", err)
	}

	cleanup := func() {
		cancel()
		select {
		case <-runErr:
		case <-time.After(3 * time.Second):
			t.Errorf("Run did not exit within 3s after cancel")
		}
	}
	return imp, nc, cleanup
}

func TestEndToEndHappyPath(t *testing.T) {
	imp, nc, cleanup := startImp(t, echoSpec(
		func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			return imps.Think(decoded, e)
		}))
	defer cleanup()

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("actions.out", func(m *nats.Msg) { got <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("messages.in", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-got:
		if string(data) != "hello" {
			t.Fatalf("got %q, want %q", data, "hello")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for action; metrics=%+v", imp.Metrics())
	}

	m := imp.Metrics()
	if m.ThinksDispatched == 0 {
		t.Fatalf("expected ThinksDispatched >= 1, got %+v", m)
	}
}

func TestDecodeFailureSkipsAwareness(t *testing.T) {
	awarenessCalls := 0
	spec := echoSpec(
		func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			awarenessCalls++
			return imps.Think(decoded, e)
		})
	spec.Channels[0].Decode = func(msg imps.Message) (any, error) {
		if string(msg.Data) == "bad" {
			return nil, errors.New("decode failed")
		}
		return string(msg.Data), nil
	}
	imp, nc, cleanup := startImp(t, spec)
	defer cleanup()

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("actions.out", func(m *nats.Msg) { got <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	// First message: decode fails. Awareness must NOT be invoked.
	if err := nc.Publish("messages.in", []byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	// Second message: decode succeeds.
	if err := nc.Publish("messages.in", []byte("ok")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-got:
		if string(data) != "ok" {
			t.Fatalf("got %q want %q", data, "ok")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout; metrics=%+v", imp.Metrics())
	}

	m := imp.Metrics()
	if m.DecodeFailures != 1 {
		t.Fatalf("expected 1 decode failure, got %d", m.DecodeFailures)
	}
	if awarenessCalls != 1 {
		t.Fatalf("awareness should be invoked exactly once (the OK message), got %d", awarenessCalls)
	}
}

func TestExtractionFailureSkipsAwareness(t *testing.T) {
	awarenessCalls := 0
	spec := echoSpec(
		func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			awarenessCalls++
			return imps.Think(decoded, e)
		})
	spec.Channels[0].ExtractEntity = func(decoded any) (imps.Entity, error) {
		if decoded.(string) == "noentity" {
			return "", nil
		}
		return imps.Entity("singleton"), nil
	}
	imp, nc, cleanup := startImp(t, spec)
	defer cleanup()

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("actions.out", func(m *nats.Msg) { got <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("messages.in", []byte("noentity")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("messages.in", []byte("good")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-got:
		if string(data) != "good" {
			t.Fatalf("got %q want %q", data, "good")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout; metrics=%+v", imp.Metrics())
	}

	m := imp.Metrics()
	if m.ExtractionFailures != 1 {
		t.Fatalf("expected 1 extraction failure, got %d", m.ExtractionFailures)
	}
	if awarenessCalls != 1 {
		t.Fatalf("awareness should be invoked exactly once, got %d", awarenessCalls)
	}
}

func TestAwarenessPanicRecovers(t *testing.T) {
	var awarenessCalls int
	awareness := func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
		awarenessCalls++
		if decoded.(string) == "panic" {
			panic("oh no")
		}
		return imps.Think(decoded, e)
	}
	imp, nc, cleanup := startImp(t, echoSpec(awareness))
	defer cleanup()

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("actions.out", func(m *nats.Msg) { got <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("messages.in", []byte("panic")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("messages.in", []byte("after-panic")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-got:
		if string(data) != "after-panic" {
			t.Fatalf("got %q want %q", data, "after-panic")
		}
	case <-time.After(time.Second):
		t.Fatalf("dispatch goroutine likely dead; metrics=%+v", imp.Metrics())
	}

	m := imp.Metrics()
	if m.AwarenessPanics != 1 {
		t.Fatalf("expected 1 awareness panic, got %d", m.AwarenessPanics)
	}
	// Reasoning should NOT have run for the panicking message: only one
	// successful awareness → one Think → one reasoning invocation.
	if m.ThinksDispatched != 1 {
		t.Fatalf("expected exactly 1 Think, got %d", m.ThinksDispatched)
	}
}
