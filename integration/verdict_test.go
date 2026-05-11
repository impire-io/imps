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

// noteSpec returns an imp with a configurable awareness function and a
// recording OnNote hook. Reasoning publishes "reasoned" so tests can
// observe whether reasoning ran.
func noteSpec(awareness harness.AwarenessFn, onNote func(harness.Entity, any)) harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "verdict-test",
		Version: "0.1.0",
		Channels: []harness.ChannelSpec{{
			Name:   "inbound",
			Source: harness.SubjectSource{Subject: "messages.in"},
			Decode: func(msg harness.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(decoded any) (harness.Entity, error) {
				return harness.Entity("singleton"), nil
			},
		}},
		Awareness: awareness,
		Reasoning: func(ctx context.Context, _ any, _ harness.Entity, r harness.ReasoningContext) error {
			return r.Publish(ctx, "actions.out", []byte("reasoned"))
		},
		OnNote:  onNote,
		Actions: []string{"actions.out"},
	}
}

func TestIgnoreVerdict(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	notes := make(chan any, 4)
	spec := noteSpec(
		func(_ context.Context, _ any, _ harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Ignore()
		},
		func(_ harness.Entity, p any) { notes <- p },
	)

	imp, err := harness.NewImp(spec, nc, harness.WithSubjectPrefix("test"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	// Watch actions.out to confirm no reasoning was queued.
	got := make(chan struct{}, 1)
	if _, err := nc.Subscribe("test.actions.out", func(*nats.Msg) { got <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("test.messages.in", []byte("ignore-me")); err != nil {
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
		t.Fatalf("Ignore must not queue reasoning")
	default:
	}

	m := imp.Metrics()
	if m.IgnoredVerdicts == 0 {
		t.Fatalf("expected IgnoredVerdicts >= 1, got %+v", m)
	}
	if m.WakesDispatched != 0 || m.NotesDelivered != 0 {
		t.Fatalf("unexpected counters: %+v", m)
	}
}

func TestNoteVerdict(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	gotNote := make(chan any, 1)
	spec := noteSpec(
		func(_ context.Context, decoded any, _ harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Note(decoded)
		},
		func(_ harness.Entity, p any) { gotNote <- p },
	)

	imp, err := harness.NewImp(spec, nc, harness.WithSubjectPrefix("test"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	gotAction := make(chan struct{}, 1)
	if _, err := nc.Subscribe("test.actions.out", func(*nats.Msg) { gotAction <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("test.messages.in", []byte("payload")); err != nil {
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

	// Ensure reasoning didn't run.
	time.Sleep(100 * time.Millisecond)
	select {
	case <-gotAction:
		t.Fatal("Note must not queue reasoning")
	default:
	}

	m := imp.Metrics()
	if m.NotesDelivered == 0 {
		t.Fatalf("expected NotesDelivered >= 1, got %+v", m)
	}
	if m.WakesDispatched != 0 {
		t.Fatalf("Note must not increment WakesDispatched: %+v", m)
	}
}

func TestWakeVerdictAsync(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	reasoningStarted := make(chan struct{})
	releaseReasoning := make(chan struct{})

	var dispatched atomic.Bool
	awareness := func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
		// dispatched is set to true when awareness is RETURNING — proves
		// dispatch did not block on reasoning.
		defer dispatched.Store(true)
		return harness.Wake(decoded, e)
	}

	spec := noteSpec(awareness, nil)
	spec.Reasoning = func(ctx context.Context, reason any, _ harness.Entity, r harness.ReasoningContext) error {
		close(reasoningStarted)
		<-releaseReasoning
		return r.Publish(ctx, "actions.out", []byte(reason.(string)))
	}

	imp, err := harness.NewImp(spec, nc, harness.WithSubjectPrefix("test"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	if err := nc.Publish("test.messages.in", []byte("hi")); err != nil {
		t.Fatal(err)
	}

	// Reasoning must start (proves Wake queued it) but dispatch returns
	// before reasoning completes (we never released it).
	select {
	case <-reasoningStarted:
	case <-time.After(time.Second):
		t.Fatal("reasoning never started")
	}
	if !dispatched.Load() {
		t.Fatal("dispatch did not return before reasoning observed")
	}

	close(releaseReasoning)
	// Give reasoning room to complete and the WaitGroup to drain.
	time.Sleep(100 * time.Millisecond)

	m := imp.Metrics()
	if m.WakesDispatched == 0 {
		t.Fatalf("expected WakesDispatched >= 1, got %+v", m)
	}
}

// waitReady polls Identity until SubjectPrefix is non-empty (Run has
// installed the runtime). Helps avoid races between Run's startup and
// publishes from the test.
func waitReady(t *testing.T, imp *harness.Imp) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if imp.Identity().SubjectPrefix != "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("imp not ready within 2s")
}
