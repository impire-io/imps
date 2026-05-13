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

// noteSpec returns an imp with a configurable awareness function and a
// recording OnNote hook. Thinking publishes "reasoned" so tests can
// observe whether thinking ran.
func noteSpec(awareness imps.AwarenessFn, onNote func(imps.Entity, any)) imps.ImpSpec {
	return imps.ImpSpec{
		Name:    "verdict-test",
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
		Thinking: func(ctx context.Context, _ any, _ imps.Entity, r imps.ThinkingContext) error {
			return r.Publish(ctx, "actions.out", []byte("reasoned"))
		},
		OnNote: onNote,
	}
}

func TestIgnoreVerdict(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	notes := make(chan any, 4)
	spec := noteSpec(
		func(_ context.Context, _ any, _ imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			return imps.Ignore()
		},
		func(_ imps.Entity, p any) { notes <- p },
	)

	imp, err := imps.NewImp(spec, nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	// Watch actions.out to confirm no thinking was queued.
	got := make(chan struct{}, 1)
	if _, err := nc.Subscribe("actions.out", func(*nats.Msg) { got <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("messages.in", []byte("ignore-me")); err != nil {
		t.Fatal(err)
	}

	// Allow processing time, then assert no Note and no action.
	time.Sleep(150 * time.Millisecond)
	select {
	case p := <-notes:
		t.Fatalf("Ignore must not invoke OnNote, got %v", p)
	default:
	}
	select {
	case <-got:
		t.Fatalf("Ignore must not queue thinking")
	default:
	}

	m := imp.Metrics()
	if m.IgnoredVerdicts == 0 {
		t.Fatalf("expected IgnoredVerdicts >= 1, got %+v", m)
	}
	if m.ThinksDispatched != 0 || m.NotesDelivered != 0 {
		t.Fatalf("unexpected counters: %+v", m)
	}
}

func TestNoteVerdict(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	gotNote := make(chan any, 1)
	spec := noteSpec(
		func(_ context.Context, decoded any, _ imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			return imps.Note(decoded)
		},
		func(_ imps.Entity, p any) { gotNote <- p },
	)

	imp, err := imps.NewImp(spec, nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	gotAction := make(chan struct{}, 1)
	if _, err := nc.Subscribe("actions.out", func(*nats.Msg) { gotAction <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("messages.in", []byte("payload")); err != nil {
		t.Fatal(err)
	}

	select {
	case p := <-gotNote:
		if s, _ := p.(string); s != "payload" {
			t.Fatalf("OnNote got %v want %q", p, "payload")
		}
	case <-time.After(time.Second):
		t.Fatal("OnNote not invoked within 1s")
	}

	// Ensure thinking didn't run.
	time.Sleep(100 * time.Millisecond)
	select {
	case <-gotAction:
		t.Fatal("Note must not queue thinking")
	default:
	}

	m := imp.Metrics()
	if m.NotesDelivered == 0 {
		t.Fatalf("expected NotesDelivered >= 1, got %+v", m)
	}
	if m.ThinksDispatched != 0 {
		t.Fatalf("Note must not increment ThinksDispatched: %+v", m)
	}
}

func TestThinkVerdictAsync(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	thinkingStarted := make(chan struct{})
	releaseThinking := make(chan struct{})

	var dispatched atomic.Bool
	awareness := func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
		// dispatched is set to true when awareness is RETURNING — proves
		// dispatch did not block on thinking.
		defer dispatched.Store(true)
		return imps.Think(decoded, e)
	}

	spec := noteSpec(awareness, nil)
	spec.Thinking = func(ctx context.Context, reason any, _ imps.Entity, r imps.ThinkingContext) error {
		close(thinkingStarted)
		<-releaseThinking
		return r.Publish(ctx, "actions.out", []byte(reason.(string)))
	}

	imp, err := imps.NewImp(spec, nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	if err := nc.Publish("messages.in", []byte("hi")); err != nil {
		t.Fatal(err)
	}

	// Thinking must start (proves Think queued it) but dispatch returns
	// before thinking completes (we never released it).
	select {
	case <-thinkingStarted:
	case <-time.After(time.Second):
		t.Fatal("thinking never started")
	}
	if !dispatched.Load() {
		t.Fatal("dispatch did not return before thinking observed")
	}

	close(releaseThinking)
	// Give thinking room to complete and the WaitGroup to drain.
	time.Sleep(100 * time.Millisecond)

	m := imp.Metrics()
	if m.ThinksDispatched == 0 {
		t.Fatalf("expected ThinksDispatched >= 1, got %+v", m)
	}
}

// waitReady polls until Imp.Ready() returns true (Run has installed
// the runtime and registered subscriptions). Helps avoid races between
// Run's startup and publishes from the test.
func waitReady(t *testing.T, imp *imps.Imp) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if imp.Ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("imp not ready within 2s")
}
