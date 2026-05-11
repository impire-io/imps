package integration_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/imps/harness"
	"github.com/impire-io/imps/testutil/natstest"
)

// streamSetup brings up an embedded NATS server with JetStream, creates a
// stream over the given source subject, and returns the connection plus
// teardown.
func streamSetup(t *testing.T, streamName, source string) (*nats.Conn, jetstream.JetStream) {
	t.Helper()
	srv := natstest.New(t)
	js := srv.JetStream(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{source},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { nc.Close() })
	return nc, js
}

func streamSpec(awareness harness.AwarenessFn, durable string, reasoning harness.ReasoningFn) harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "stream-imp",
		Version: "0.1.0",
		Channels: []harness.ChannelSpec{{
			Name: "orders",
			Source: harness.StreamSource{
				Stream:        "ORDERS",
				FilterSubject: "orders.created",
				Durable:       durable,
			},
			Decode: func(msg harness.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(decoded any) (harness.Entity, error) {
				return harness.Entity("singleton"), nil
			},
		}},
		Awareness: awareness,
		Reasoning: reasoning,
		Actions:   []string{"actions.out"},
	}
}

func runStreamImp(t *testing.T, nc *nats.Conn, spec harness.ImpSpec, opts ...harness.Option) (*harness.Imp, func()) {
	t.Helper()
	defaults := []harness.Option{
		harness.WithSubjectPrefix("test"),
		harness.WithDrainWindow(1 * time.Second),
	}
	imp, err := harness.NewImp(spec, nc, append(defaults, opts...)...)
	if err != nil {
		t.Fatalf("NewImp: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if imp.Identity().SubjectPrefix != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return imp, func() {
		cancel()
		select {
		case <-runErr:
		case <-time.After(3 * time.Second):
			t.Errorf("Run did not exit within 3s")
		}
	}
}

func TestStreamChannelDurableHappyPath(t *testing.T) {
	// stream subject must match the resolved channel subject — non-platform
	// mode prefix "test" + channel "orders.created" → "test.orders.created".
	nc, _ := streamSetup(t, "ORDERS", "test.orders.created")

	// First run: publish, observe action.
	publishedAction := make(chan []byte, 4)
	if _, err := nc.Subscribe("test.actions.out", func(m *nats.Msg) { publishedAction <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	spec := streamSpec(
		func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Wake(decoded, e)
		},
		"echo-orders",
		func(ctx context.Context, reason any, _ harness.Entity, r harness.ReasoningContext) error {
			return r.Publish(ctx, "actions.out", []byte(reason.(string)))
		},
	)

	imp1, cleanup1 := runStreamImp(t, nc, spec)
	if err := nc.Publish("test.orders.created", []byte("first")); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-publishedAction:
		if string(data) != "first" {
			t.Fatalf("got %q want first", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("first message not processed; metrics=%+v", imp1.Metrics())
	}
	cleanup1()

	// Second run: same durable, publish a fresh message; durable consumer
	// resumes (no double delivery of "first") and processes the new one.
	imp2, cleanup2 := runStreamImp(t, nc, spec)
	defer cleanup2()
	if err := nc.Publish("test.orders.created", []byte("second")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-publishedAction:
		if string(data) != "second" {
			t.Fatalf("got %q want second", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("second message not processed; metrics=%+v", imp2.Metrics())
	}

	// Ensure no spurious redelivery of "first".
	select {
	case data := <-publishedAction:
		t.Fatalf("unexpected redelivery: %q", data)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestEphemeralConsumerLifecycle(t *testing.T) {
	nc, js := streamSetup(t, "ORDERS", "test.orders.created")

	consumerCountBefore := countConsumers(t, js, "ORDERS")

	spec := streamSpec(
		func(_ context.Context, _ any, _ harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Ignore()
		},
		"", // ephemeral
		func(_ context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error { return nil },
	)
	_, cleanup := runStreamImp(t, nc, spec)

	// While running, an ephemeral consumer should exist.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if countConsumers(t, js, "ORDERS") == consumerCountBefore+1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := countConsumers(t, js, "ORDERS"); got != consumerCountBefore+1 {
		t.Fatalf("expected %d consumers (one ephemeral), got %d", consumerCountBefore+1, got)
	}

	cleanup()

	// After clean shutdown, the ephemeral consumer must be gone.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if countConsumers(t, js, "ORDERS") == consumerCountBefore {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := countConsumers(t, js, "ORDERS"); got != consumerCountBefore {
		t.Fatalf("ephemeral consumer not torn down: expected %d, got %d", consumerCountBefore, got)
	}
}

func TestStreamNotFound(t *testing.T) {
	srv := natstest.New(t)
	_ = srv.JetStream(t) // enable JetStream but don't create the stream
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })

	spec := streamSpec(
		func(_ context.Context, _ any, _ harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Ignore()
		},
		"any",
		func(_ context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error { return nil },
	)

	imp, err := harness.NewImp(spec, nc, harness.WithSubjectPrefix("test"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := imp.Run(ctx)

	var sn *harness.ErrStreamNotFound
	if !errors.As(runErr, &sn) || sn.Stream != "ORDERS" {
		t.Fatalf("expected ErrStreamNotFound{ORDERS}, got %v", runErr)
	}
}

func TestConsumerIncompatible(t *testing.T) {
	nc, js := streamSetup(t, "ORDERS", "test.orders.created")

	// Pre-create a durable consumer with a DIFFERENT filter subject.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := js.CreateOrUpdateConsumer(ctx, "ORDERS", jetstream.ConsumerConfig{
		Durable:       "echo-orders",
		FilterSubject: "test.orders.different",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("pre-create consumer: %v", err)
	}

	spec := streamSpec(
		func(_ context.Context, _ any, _ harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Ignore()
		},
		"echo-orders",
		func(_ context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error { return nil },
	)

	imp, err := harness.NewImp(spec, nc, harness.WithSubjectPrefix("test"))
	if err != nil {
		t.Fatal(err)
	}
	rctx, rcancel := context.WithCancel(context.Background())
	defer rcancel()
	runErr := imp.Run(rctx)

	var ci *harness.ErrConsumerIncompatible
	if !errors.As(runErr, &ci) || ci.Consumer != "echo-orders" {
		t.Fatalf("expected ErrConsumerIncompatible{echo-orders}, got %v", runErr)
	}
	if ci.Diff == "" {
		t.Fatalf("Diff must be populated, got empty")
	}
}

func TestAckAtAwarenessCompletion(t *testing.T) {
	nc, js := streamSetup(t, "ORDERS", "test.orders.created")

	spec := streamSpec(
		func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Wake(decoded, e)
		},
		"echo-orders",
		func(ctx context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error {
			// Hold reasoning to prove ack happens BEFORE reasoning completes.
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
			}
			return nil
		},
	)

	_, cleanup := runStreamImp(t, nc, spec)
	defer cleanup()

	if err := nc.Publish("test.orders.created", []byte("payload")); err != nil {
		t.Fatal(err)
	}

	// Within 200 ms (well before reasoning completes at 500 ms), the
	// consumer's pending count must drop to zero.
	deadline := time.Now().Add(300 * time.Millisecond)
	pending := 1
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		stream, err := js.Stream(ctx, "ORDERS")
		if err != nil {
			cancel()
			t.Fatalf("Stream: %v", err)
		}
		c, err := stream.Consumer(ctx, "echo-orders")
		cancel()
		if err != nil {
			continue
		}
		info, err := c.Info(context.Background())
		if err != nil {
			continue
		}
		pending = info.NumAckPending
		if pending == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pending != 0 {
		t.Fatalf("ack should be sent before reasoning completes; pending=%d", pending)
	}
}

func TestStreamNakOnFailures(t *testing.T) {
	nc, _ := streamSetup(t, "ORDERS", "test.orders.created")

	var awarenessCalls atomic.Int32
	spec := streamSpec(
		func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			awarenessCalls.Add(1)
			s, _ := decoded.(string)
			if s == "panic" {
				panic("awareness panic")
			}
			return harness.Wake(decoded, e)
		},
		"echo-orders",
		func(ctx context.Context, _ any, _ harness.Entity, r harness.ReasoningContext) error {
			return r.Publish(ctx, "actions.out", []byte("ok"))
		},
	)
	spec.Channels[0].Decode = func(msg harness.Message) (any, error) {
		s := string(msg.Data)
		if s == "decode-fail" {
			return nil, errors.New("decode failed")
		}
		return s, nil
	}
	spec.Channels[0].ExtractEntity = func(decoded any) (harness.Entity, error) {
		if decoded.(string) == "extract-fail" {
			return "", nil
		}
		return harness.Entity("singleton"), nil
	}
	// MaxDeliver=1 so each NAK does NOT trigger an infinite redelivery
	// loop and we can finish the test.
	spec.Channels[0].Source = harness.StreamSource{
		Stream:        "ORDERS",
		FilterSubject: "orders.created",
		Durable:       "echo-orders",
		ConsumerConfig: harness.ConsumerConfig{
			MaxDeliver: 1,
		},
	}

	imp, cleanup := runStreamImp(t, nc, spec)
	defer cleanup()

	for _, payload := range []string{"decode-fail", "extract-fail", "panic"} {
		if err := nc.Publish("test.orders.created", []byte(payload)); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for processing.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m := imp.Metrics()
		if m.NakTotal >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	m := imp.Metrics()
	if m.NakTotal != 3 {
		t.Fatalf("expected NakTotal=3, got %d (full=%+v)", m.NakTotal, m)
	}
	if m.DecodeFailures != 1 {
		t.Fatalf("expected DecodeFailures=1, got %d", m.DecodeFailures)
	}
	if m.ExtractionFailures != 1 {
		t.Fatalf("expected ExtractionFailures=1, got %d", m.ExtractionFailures)
	}
	if m.AwarenessPanics != 1 {
		t.Fatalf("expected AwarenessPanics=1, got %d", m.AwarenessPanics)
	}
}

func countConsumers(t *testing.T, js jetstream.JetStream, streamName string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	count := 0
	lister := stream.ConsumerNames(ctx)
	for range lister.Name() {
		count++
	}
	if err := lister.Err(); err != nil {
		t.Fatalf("ConsumerNames: %v", err)
	}
	return count
}
