package schedule

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	imps "github.com/impire-io/imps"
	"github.com/impire-io/imps/testutil/natstest"
)

type recorder struct {
	mu   sync.Mutex
	seen []Tick
}

func (r *recorder) add(t Tick) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, t)
}

func (r *recorder) countOn(subject string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, tk := range r.seen {
		if tk.Subject == subject {
			n++
		}
	}
	return n
}

func (r *recorder) snapshot() []Tick {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Tick(nil), r.seen...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func newTickImp(t *testing.T, nc *nats.Conn, rec *recorder) *imps.Imp {
	t.Helper()
	spec := imps.ImpSpec{
		Name:    "clocked",
		Version: "0.0.1",
		Channels: []imps.ChannelSpec{
			Channel("SCHED", "ticks.hb", WithDurable("hb-watch")),
			Channel("SCHED", "ticks.audit", WithDurable("audit-watch")),
		},
		Awareness: func(_ context.Context, decoded any, _ imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			rec.add(decoded.(Tick))
			return imps.Ignore()
		},
		Thinking: func(context.Context, any, imps.Entity, imps.ThinkingContext) error { return nil },
	}
	imp, err := imps.NewImp(spec, nc)
	if err != nil {
		t.Fatalf("NewImp: %v", err)
	}
	return imp
}

// TestScheduleChannel_WarmColdTTL is the research spike (episode 0008)
// productized: server-produced ticks reach awareness through the existing
// dispatch path live and as durable catch-up, with stale-tick accumulation
// governed by the schedule's TTL in both directions.
func TestScheduleChannel_WarmColdTTL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	s := natstest.New(t)
	js := s.JetStream(t)
	newScheduleStream(ctx, t, js)

	// Two schedules at 1s cadence: hb ticks expire after 2s; audit ticks
	// accumulate (the deliberate no-TTL choice).
	if err := Register(ctx, js, "schedules.hb", "@every 1s", "ticks.hb", WithTickTTL(2*time.Second)); err != nil {
		t.Fatalf("register hb: %v", err)
	}
	if err := Register(ctx, js, "schedules.audit", "@every 1s", "ticks.audit"); err != nil {
		t.Fatalf("register audit: %v", err)
	}

	// ---- Warm: live ticks through the ordinary dispatch path ----
	ncA, err := nats.Connect(s.URL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer ncA.Close()
	recA := &recorder{}
	impA := newTickImp(t, ncA, recA)
	runA := make(chan error, 1)
	go func() { runA <- impA.Run(ctx) }()
	waitFor(t, "imp ready", func() bool { return impA.Ready() })
	waitFor(t, "two live ticks per schedule", func() bool {
		return recA.countOn("ticks.hb") >= 2 && recA.countOn("ticks.audit") >= 2
	})

	// Provenance on every delivered tick (SC-001).
	for _, tk := range recA.snapshot() {
		want := map[string]string{"ticks.hb": "schedules.hb", "ticks.audit": "schedules.audit"}[tk.Subject]
		if tk.Scheduler != want {
			t.Errorf("tick on %s has Scheduler %q, want %q", tk.Subject, tk.Scheduler, want)
		}
	}
	m := impA.Metrics()
	if m.DecodeFailures != 0 || m.ExtractionFailures != 0 || m.AwarenessPanics != 0 {
		t.Errorf("unexpected failures: %+v", m)
	}
	if err := impA.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := <-runA; err != nil {
		t.Fatalf("run: %v", err)
	}
	ncA.Close()

	// ---- Cold: both schedules keep firing with no imp attached ----
	time.Sleep(5 * time.Second)

	// ---- Wake: durable catch-up, server-pruned per TTL ----
	ncB, err := nats.Connect(s.URL())
	if err != nil {
		t.Fatalf("connect B: %v", err)
	}
	defer ncB.Close()
	recB := &recorder{}
	impB := newTickImp(t, ncB, recB)
	runB := make(chan error, 1)
	go func() { runB <- impB.Run(ctx) }()
	defer func() {
		if err := impB.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown B: %v", err)
		}
		<-runB
	}()
	waitFor(t, "imp B ready", func() bool { return impB.Ready() })
	waitFor(t, "cold catch-up", func() bool {
		return recB.countOn("ticks.audit") >= 3 && recB.countOn("ticks.hb") >= 1
	})
	time.Sleep(700 * time.Millisecond) // settle: drain any interleaved live ticks

	audit := recB.countOn("ticks.audit")
	hb := recB.countOn("ticks.hb")

	// No TTL: the full cold backlog accumulated and arrived (SC-002).
	if audit < 3 {
		t.Errorf("audit (no TTL) delivered %d cold ticks, want the full backlog (>=3)", audit)
	}
	// TTL 2s: strictly fewer — only the unexpired tail, pruned server-side
	// with zero imp-side filtering (SC-002).
	if hb >= audit {
		t.Errorf("hb (TTL 2s) delivered %d, audit %d — TTL did not govern accumulation", hb, audit)
	}
	if hb < 1 || hb > 4 {
		t.Errorf("hb (TTL 2s) delivered %d cold ticks, want 1..4 (the unexpired tail)", hb)
	}
}
