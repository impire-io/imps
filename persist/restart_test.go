package persist

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	imps "github.com/impire-io/imps"
	"github.com/impire-io/imps/testutil/natstest"
)

// TestRestart_SurvivalWakeAndBeacon is the research spike (episode 0005)
// productized: a real imp mutates durable state from awareness, stops
// (write-through means stopping is always safe), and a fresh instance
// rehydrates codec-equal state with the wake hook delivering the true
// elapsed time; the Beacon restart clock measures the same gap at imp
// level.
func TestRestart_SurvivalWakeAndBeacon(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	s := natstest.New(t)
	js := s.JetStream(t)
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "imp-state"})
	if err != nil {
		t.Fatalf("kv: %v", err)
	}
	backend := KVBackend(kv)

	// ---- Instance A: awareness updates the store per event ----
	ncA, err := nats.Connect(s.URL())
	if err != nil {
		t.Fatalf("connect A: %v", err)
	}
	defer ncA.Close()
	storeA := NewStore[custState]("cust", backend)
	beaconA := NewBeacon("cust-watcher", backend)

	var processed atomic.Int64
	specA := imps.ImpSpec{
		Name:    "sleeper",
		Version: "0.0.1",
		Channels: []imps.ChannelSpec{{
			Name:   "events",
			Source: imps.SubjectSource{Subject: "persist.events.*"},
			Decode: func(m imps.Message) (any, error) {
				n, err := strconv.Atoi(string(m.Data))
				if err != nil {
					return nil, err
				}
				parts := strings.Split(m.Subject, ".")
				return struct {
					E imps.Entity
					N int
				}{imps.Entity(parts[len(parts)-1]), n}, nil
			},
			ExtractEntity: func(decoded any) (imps.Entity, error) {
				return decoded.(struct {
					E imps.Entity
					N int
				}).E, nil
			},
		}},
		Awareness: func(ctx context.Context, decoded any, entity imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			ev := decoded.(struct {
				E imps.Entity
				N int
			})
			if _, err := storeA.Update(ctx, entity, func(st custState) custState {
				st.Counter += ev.N
				return st
			}); err != nil {
				t.Errorf("update: %v", err)
			}
			processed.Add(1)
			return imps.Ignore()
		},
		Thinking: func(context.Context, any, imps.Entity, imps.ThinkingContext) error { return nil },
	}
	impA, err := imps.NewImp(specA, ncA)
	if err != nil {
		t.Fatalf("NewImp A: %v", err)
	}
	runA := make(chan error, 1)
	go func() { runA <- impA.Run(ctx) }()
	waitReady(t, impA)

	for _, n := range []string{"1", "2", "3"} {
		if err := ncA.Publish("persist.events.cust-1", []byte(n)); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	deadline := time.Now().Add(15 * time.Second)
	for processed.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if processed.Load() != 3 {
		t.Fatalf("processed %d events, want 3", processed.Load())
	}

	// Stopping is safe by construction: stamp the beacon, shut down,
	// nothing to flush.
	if err := beaconA.Stamp(ctx); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	stopped := time.Now()
	if err := impA.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown A: %v", err)
	}
	if err := <-runA; err != nil {
		t.Fatalf("run A: %v", err)
	}
	ncA.Close()

	const sleepFor = 400 * time.Millisecond
	time.Sleep(sleepFor)

	// ---- Instance B: fresh store + beacon against the same backend ----
	var wakes atomic.Int64
	var wakeElapsed atomic.Int64
	storeB := NewStore[custState]("cust", backend,
		WithWake(func(_ imps.Entity, elapsed time.Duration, st custState) custState {
			wakes.Add(1)
			wakeElapsed.Store(int64(elapsed))
			st.IdleMs = elapsed.Milliseconds()
			return st
		}))
	beaconB := NewBeacon("cust-watcher", backend)

	// Imp-level: the pre-Run gate reading.
	slept, ok, err := beaconB.SleptFor(ctx)
	if err != nil || !ok {
		t.Fatalf("slept-for = (%v, %v, %v), want a measured sleep", slept, ok, err)
	}
	wall := time.Since(stopped) + time.Second
	if slept < sleepFor || slept > wall {
		t.Errorf("beacon slept = %v, want within [%v, %v]", slept, sleepFor, wall)
	}

	// Entity-level: codec-equal state, wake exactly once with true elapsed,
	// advancement visible pre-observation.
	got, err := storeB.Get(ctx, "cust-1")
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if got.Counter != 6 {
		t.Errorf("Counter = %d, want 6 (1+2+3 from before the restart)", got.Counter)
	}
	if wakes.Load() != 1 {
		t.Errorf("wake fired %d times, want exactly 1", wakes.Load())
	}
	e := time.Duration(wakeElapsed.Load())
	if e < sleepFor || e > time.Since(stopped)+time.Second {
		t.Errorf("wake elapsed = %v, want ≥ %v and wall-clock-bounded", e, sleepFor)
	}
	if got.IdleMs != e.Milliseconds() {
		t.Errorf("IdleMs = %d, want the wake-delivered %d", got.IdleMs, e.Milliseconds())
	}
	// Resident hit: no re-fire.
	if _, err := storeB.Get(ctx, "cust-1"); err != nil {
		t.Fatalf("second get: %v", err)
	}
	if wakes.Load() != 1 {
		t.Errorf("resident hit re-fired the wake (%d)", wakes.Load())
	}
}

func waitReady(t *testing.T, imp *imps.Imp) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if imp.Ready() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("imp not ready in time")
}
