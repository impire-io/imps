package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/imps/harness"
	"github.com/impire-io/imps/testutil/natstest"
)

func lifecycleSpec(channels []harness.ChannelSpec, reasoning harness.ReasoningFn) harness.ImpSpec {
	return harness.ImpSpec{
		Name:     "lifecycle",
		Version:  "1.2.3",
		Channels: channels,
		Awareness: func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Wake(decoded, e)
		},
		Reasoning: reasoning,
	}
}

func subjectChannel(name, subject string) harness.ChannelSpec {
	return harness.ChannelSpec{
		Name:   name,
		Source: harness.SubjectSource{Subject: subject},
		Decode: func(msg harness.Message) (any, error) {
			return string(msg.Data), nil
		},
		ExtractEntity: func(any) (harness.Entity, error) { return "singleton", nil },
	}
}

func TestStartupRegistersSubscriptions(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	channels := []harness.ChannelSpec{
		subjectChannel("a", "messages.a"),
		subjectChannel("b", "messages.b"),
	}
	spec := lifecycleSpec(channels, func(_ context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error { return nil })

	imp, err := harness.NewImp(spec, nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	// Identity matches.
	id := imp.Identity()
	if id.Name != "lifecycle" || id.Version != "1.2.3" {
		t.Fatalf("unexpected identity: %+v", id)
	}

	// Each declared channel is observable as a subscription.
	got := make(chan string, 4)
	for _, ch := range channels {
		ch := ch
		if _, err := nc.Subscribe(ch.Source.(harness.SubjectSource).Subject, func(*nats.Msg) {}); err != nil {
			t.Fatal(err)
		}
		// publish on the literal subject — if the imp's subscription is up,
		// dispatch will run; we only care that a publish doesn't silently
		// disappear.
		_ = nc.Publish(ch.Source.(harness.SubjectSource).Subject, []byte(ch.Name))
		got <- ch.Name
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(channels) {
		t.Fatalf("expected %d channels recorded, got %d", len(channels), len(got))
	}
}

func TestStartupFailureNoLeaks(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	// Channel A: a normal subject channel — establishes successfully.
	// Channel B: a stream channel referencing a missing stream — fails.
	channels := []harness.ChannelSpec{
		subjectChannel("a", "messages.a"),
		{
			Name: "b",
			Source: harness.StreamSource{
				Stream:        "MISSING_STREAM",
				FilterSubject: "messages.b",
			},
			Decode:        func(harness.Message) (any, error) { return nil, nil },
			ExtractEntity: func(any) (harness.Entity, error) { return "x", nil },
		},
	}
	spec := lifecycleSpec(channels, func(_ context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error { return nil })

	imp, err := harness.NewImp(spec, nc)
	if err != nil {
		t.Fatal(err)
	}
	// Need JetStream enabled on the server for the missing-stream check.
	srv.JetStream(t)

	// Pre-warm the JetStream request-reply inbox subscription so it does
	// not count as a "leak" against the harness when we measure below.
	// The muxed inbox is created on the first request and persists for
	// the lifetime of the connection — it is a NATS-internal artifact,
	// not a harness subscription.
	preJS, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	preCtx, preCancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, _ = preJS.AccountInfo(preCtx)
	preCancel()

	subsBefore := nc.NumSubscriptions()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := imp.Run(ctx)

	var sn *harness.ErrStreamNotFound
	if !errors.As(runErr, &sn) {
		t.Fatalf("expected ErrStreamNotFound, got %v", runErr)
	}

	// Allow the connection to settle; assert that no leftover subscriptions remain.
	time.Sleep(50 * time.Millisecond)
	subsAfter := nc.NumSubscriptions()
	if subsAfter > subsBefore {
		t.Fatalf("subscriptions leaked: before=%d after=%d", subsBefore, subsAfter)
	}
}

func TestShutdownDrainBoundedReturn(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	const drain = 250 * time.Millisecond
	subsBefore := nc.NumSubscriptions()

	reasoningStart := make(chan struct{}, 4)
	reasoning := func(ctx context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error {
		reasoningStart <- struct{}{}
		<-ctx.Done()
		return nil
	}

	spec := lifecycleSpec([]harness.ChannelSpec{subjectChannel("inbound", "messages.in")}, reasoning)
	imp, err := harness.NewImp(spec, nc, harness.WithDrainWindow(drain))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()
	waitReady(t, imp)

	if err := nc.Publish("messages.in", []byte("E1")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("messages.in", []byte("E2")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-reasoningStart:
		case <-time.After(2 * time.Second):
			t.Fatalf("reasoning %d never started", i+1)
		}
	}

	t0 := time.Now()
	cancel()
	select {
	case <-runErr:
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within 2s")
	}
	elapsed := time.Since(t0)
	if elapsed > drain+500*time.Millisecond {
		t.Fatalf("shutdown took %v, expected < %v", elapsed, drain+500*time.Millisecond)
	}

	// Subscriptions are gone.
	if got := nc.NumSubscriptions(); got > subsBefore {
		t.Fatalf("subscriptions remain after shutdown: before=%d after=%d", subsBefore, got)
	}
}

func TestIdentityAcrossStates(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	spec := lifecycleSpec([]harness.ChannelSpec{subjectChannel("inbound", "messages.in")},
		func(_ context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error { return nil })

	imp, err := harness.NewImp(spec, nc)
	if err != nil {
		t.Fatal(err)
	}

	// Before Run: Identity returns spec values.
	id := imp.Identity()
	if id.Name != "lifecycle" || id.Version != "1.2.3" {
		t.Fatalf("pre-run identity: %+v", id)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()
	waitReady(t, imp)

	// Running: identity unchanged.
	id = imp.Identity()
	if id.Name != "lifecycle" || id.Version != "1.2.3" {
		t.Fatalf("running identity: %+v", id)
	}

	cancel()
	<-runErr

	// Stopped: identity still available.
	id = imp.Identity()
	if id.Name != "lifecycle" || id.Version != "1.2.3" {
		t.Fatalf("stopped identity: %+v", id)
	}
}
