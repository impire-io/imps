package integration_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps/harness"
	"github.com/impire-io/imps/testutil/natstest"
)

type counter struct{ n int }

// memorySpec returns an imp with a single state shape "counter" of the
// given cap. The awareness function is supplied by the test so each
// scenario can probe state behavior without being tangled in stream/sub
// plumbing.
func memorySpec(cap int, awareness harness.AwarenessFn, onNote func(harness.Entity, any)) harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "memory-test",
		Version: "0.1.0",
		States: []harness.StateShape{{
			Name:    "counter",
			Factory: func() any { return &counter{} },
			Cap:     cap,
		}},
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
		Awareness: awareness,
		Reasoning: func(_ context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error { return nil },
		OnNote:    onNote,
	}
}

func bringUp(t *testing.T, spec harness.ImpSpec, opts ...harness.Option) (*harness.Imp, *nats.Conn, func()) {
	t.Helper()
	srv := natstest.New(t)
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })
	defaults := []harness.Option{harness.WithDrainWindow(1 * time.Second)}
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

func TestPerEntityStateConsistency(t *testing.T) {
	const N = 5
	counters := sync.Map{}

	awareness := func(_ context.Context, _ any, e harness.Entity, a harness.AwarenessContext) harness.Verdict {
		ref, err := a.State("counter", e)
		if err != nil {
			return harness.Note(err)
		}
		_ = ref.Update(func(v any) any {
			c := v.(*counter)
			c.n++
			return c
		})
		val := ref.Get().(*counter).n
		counters.Store(string(e), val)
		return harness.Ignore()
	}

	imp, nc, cleanup := bringUp(t, memorySpec(N, awareness, nil))
	defer cleanup()

	// Drive each entity 3 times.
	for round := 0; round < 3; round++ {
		for i := 0; i < N; i++ {
			if err := nc.Publish("messages.in", []byte(strconv.Itoa(i))); err != nil {
				t.Fatal(err)
			}
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if imp.Metrics().IgnoredVerdicts >= 3*N {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	for i := 0; i < N; i++ {
		v, ok := counters.Load(strconv.Itoa(i))
		if !ok {
			t.Fatalf("entity %d not seen", i)
		}
		if v.(int) != 3 {
			t.Fatalf("entity %d expected count 3, got %d", i, v.(int))
		}
	}
}

func TestCapExceededOnNewEntity(t *testing.T) {
	const N = 3
	notes := make(chan any, 8)

	awareness := func(_ context.Context, _ any, e harness.Entity, a harness.AwarenessContext) harness.Verdict {
		_, err := a.State("counter", e)
		if err != nil {
			return harness.Note(err)
		}
		return harness.Ignore()
	}

	imp, nc, cleanup := bringUp(t, memorySpec(N, awareness, func(_ harness.Entity, p any) { notes <- p }))
	defer cleanup()

	// Drive N+1 distinct entities.
	for i := 0; i <= N; i++ {
		if err := nc.Publish("messages.in", []byte(strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for the cap-exceeded note to arrive.
	select {
	case p := <-notes:
		var capErr *harness.ErrCapExceeded
		if !errors.As(p.(error), &capErr) {
			t.Fatalf("expected ErrCapExceeded, got %v", p)
		}
		if capErr.Shape != "counter" || capErr.Count != N {
			t.Fatalf("unexpected cap err: %+v", capErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no cap-exceeded note within 2s; metrics=%+v", imp.Metrics())
	}
}

func TestExistingSlotsAfterCap(t *testing.T) {
	const N = 1
	awareness := func(_ context.Context, _ any, e harness.Entity, a harness.AwarenessContext) harness.Verdict {
		ref, err := a.State("counter", e)
		if err != nil {
			return harness.Note(err)
		}
		_ = ref.Update(func(v any) any {
			c := v.(*counter)
			c.n++
			return c
		})
		return harness.Ignore()
	}

	imp, nc, cleanup := bringUp(t, memorySpec(N, awareness, nil))
	defer cleanup()

	// Allocate the only slot.
	if err := nc.Publish("messages.in", []byte("only")); err != nil {
		t.Fatal(err)
	}
	// Trigger cap-exceeded.
	if err := nc.Publish("messages.in", []byte("over")); err != nil {
		t.Fatal(err)
	}
	// Existing entity should keep working.
	for i := 0; i < 3; i++ {
		if err := nc.Publish("messages.in", []byte("only")); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		// 4 successful Ignore (only x4) + 1 Note (over)
		if imp.Metrics().IgnoredVerdicts >= 4 && imp.Metrics().NotesDelivered >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	m := imp.Metrics()
	if m.IgnoredVerdicts < 4 {
		t.Fatalf("expected at least 4 ignored verdicts, got %d (%+v)", m.IgnoredVerdicts, m)
	}
	if m.NotesDelivered < 1 {
		t.Fatalf("expected at least 1 note (cap exceeded), got %d", m.NotesDelivered)
	}
}

func TestUnknownStateShapeError(t *testing.T) {
	notes := make(chan any, 1)
	awareness := func(_ context.Context, _ any, e harness.Entity, a harness.AwarenessContext) harness.Verdict {
		_, err := a.State("not-declared", e)
		return harness.Note(err)
	}

	_, nc, cleanup := bringUp(t, memorySpec(10, awareness, func(_ harness.Entity, p any) { notes <- p }))
	defer cleanup()

	if err := nc.Publish("messages.in", []byte("x")); err != nil {
		t.Fatal(err)
	}

	select {
	case p := <-notes:
		var unk *harness.ErrUnknownStateShape
		if !errors.As(p.(error), &unk) || unk.Shape != "not-declared" {
			t.Fatalf("expected ErrUnknownStateShape{not-declared}, got %v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no note within 1s")
	}
}

func TestConcurrentSameEntitySerialized(t *testing.T) {
	const N = 1
	const messages = 100

	finalSeen := make(chan int, 1)
	var inflight atomic.Int32

	awareness := func(_ context.Context, _ any, e harness.Entity, a harness.AwarenessContext) harness.Verdict {
		// Simulate two awareness paths for the same entity racing on Update.
		ref, err := a.State("counter", e)
		if err != nil {
			return harness.Note(err)
		}
		_ = ref.Update(func(v any) any {
			c := v.(*counter)
			c.n++
			return c
		})
		final := ref.Get().(*counter).n
		if int(inflight.Add(1)) >= messages {
			select {
			case finalSeen <- final:
			default:
			}
		}
		return harness.Ignore()
	}

	imp, nc, cleanup := bringUp(t, memorySpec(N, awareness, nil))
	defer cleanup()

	// Burst of messages all on the same entity.
	var wg sync.WaitGroup
	for i := 0; i < messages; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := nc.Publish("messages.in", []byte("same")); err != nil {
				t.Errorf("publish: %v", err)
			}
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(3 * time.Second)
	var observed int
	for time.Now().Before(deadline) {
		select {
		case observed = <-finalSeen:
		default:
		}
		if observed >= messages {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if observed < messages {
		t.Fatalf("expected at least %d updates serialized, observed %d (metrics=%+v)", messages, observed, imp.Metrics())
	}
}
