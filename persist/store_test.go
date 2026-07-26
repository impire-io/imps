package persist

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	imps "github.com/impire-io/imps"
)

// custState is the codec-equipped, time-dependent state used across the
// store tests.
type custState struct {
	Counter int   `json:"counter"`
	IdleMs  int64 `json:"idle_ms"`
}

// fakeClock is the injected time source for deterministic elapsed tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestUpdate_WriteThrough(t *testing.T) {
	ctx := context.Background()
	b := newMemBackend()
	s := NewStore[custState]("cust", b)

	if _, err := s.Update(ctx, "c1", func(st custState) custState {
		st.Counter = 7
		return st
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	// The envelope is on the backend the moment Update returned.
	raw, err := b.Get(ctx, "cust.c1")
	if err != nil {
		t.Fatalf("backend read: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("envelope decode: %v", err)
	}
	var st custState
	if err := json.Unmarshal(env.State, &st); err != nil {
		t.Fatalf("state decode: %v", err)
	}
	if st.Counter != 7 {
		t.Errorf("durable Counter = %d, want 7", st.Counter)
	}
	if env.LastActive.IsZero() {
		t.Error("LastActive not stamped")
	}
}

func TestGet_NeverSeenIsZeroNoWake(t *testing.T) {
	ctx := context.Background()
	var wakes atomic.Int64
	s := NewStore[custState]("cust", newMemBackend(),
		WithWake(func(imps.Entity, time.Duration, custState) custState {
			wakes.Add(1)
			return custState{}
		}))

	got, err := s.Get(ctx, "fresh")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != (custState{}) {
		t.Errorf("state = %+v, want zero", got)
	}
	if wakes.Load() != 0 {
		t.Errorf("wake fired for a never-seen entity")
	}
}

func TestErrorsSurface_NeverSilentZero(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("backend down")
	s := NewStore[custState]("cust", failBackend{err: boom})

	if _, err := s.Get(ctx, "c1"); !errors.Is(err, boom) {
		t.Errorf("Get err = %v, want the backend error", err)
	}
	if _, err := s.Update(ctx, "c1", func(st custState) custState { return st }); !errors.Is(err, boom) {
		t.Errorf("Update err = %v, want the backend error", err)
	}
}

func TestDelete_OnlyRemovalPath(t *testing.T) {
	ctx := context.Background()
	b := newMemBackend()
	s := NewStore[custState]("cust", b)

	if _, err := s.Update(ctx, "c1", func(st custState) custState { st.Counter = 1; return st }); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := s.Delete(ctx, "c1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := b.Get(ctx, "cust.c1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("backend still holds deleted entity: %v", err)
	}
	if s.Resident() != 0 {
		t.Errorf("resident = %d after delete, want 0", s.Resident())
	}
	// Deleting an unknown entity is not an error.
	if err := s.Delete(ctx, "never-existed"); err != nil {
		t.Errorf("delete unknown: %v", err)
	}
}

type prefixCodec struct{}

func (prefixCodec) Marshal(v custState) ([]byte, error) {
	return append([]byte("PFX:"), []byte(strconv.Itoa(v.Counter))...), nil
}

func (prefixCodec) Unmarshal(data []byte) (custState, error) {
	n, err := strconv.Atoi(string(data[4:]))
	return custState{Counter: n}, err
}

func TestCodecOverride_NonJSONBytes(t *testing.T) {
	ctx := context.Background()
	b := newMemBackend()
	s1 := NewStore[custState]("cust", b, WithCodec[custState](prefixCodec{}))
	if _, err := s1.Update(ctx, "c1", func(st custState) custState { st.Counter = 42; return st }); err != nil {
		t.Fatalf("update: %v", err)
	}
	// A fresh store rehydrates through the same codec — the envelope carries
	// the codec's bytes opaquely (they are not JSON).
	s2 := NewStore[custState]("cust", b, WithCodec[custState](prefixCodec{}))
	got, err := s2.Get(ctx, "c1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Counter != 42 {
		t.Errorf("Counter = %d, want 42", got.Counter)
	}
}

func TestWake_ExactlyOnceWithTrueElapsed(t *testing.T) {
	ctx := context.Background()
	b := newMemBackend()
	clock := newFakeClock()

	// Writer stamps last_active at t0.
	w := NewStore[custState]("cust", b)
	w.now = clock.now
	if _, err := w.Update(ctx, "c1", func(st custState) custState { st.Counter = 6; return st }); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A fresh store reads 5 minutes later.
	clock.advance(5 * time.Minute)
	var wakes atomic.Int64
	var gotElapsed atomic.Int64
	r := NewStore[custState]("cust", b,
		WithWake(func(_ imps.Entity, elapsed time.Duration, st custState) custState {
			wakes.Add(1)
			gotElapsed.Store(int64(elapsed))
			st.IdleMs = elapsed.Milliseconds()
			return st
		}))
	r.now = clock.now

	// Pre-visibility: the fn passed to Update must already see the woken
	// state.
	if _, err := r.Update(ctx, "c1", func(st custState) custState {
		if st.IdleMs != (5 * time.Minute).Milliseconds() {
			t.Errorf("fn saw pre-wake state: IdleMs = %d", st.IdleMs)
		}
		return st
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if wakes.Load() != 1 {
		t.Fatalf("wake fired %d times, want 1", wakes.Load())
	}
	if time.Duration(gotElapsed.Load()) != 5*time.Minute {
		t.Errorf("elapsed = %v, want exactly 5m", time.Duration(gotElapsed.Load()))
	}
	// Resident hit: no re-fire.
	if _, err := r.Get(ctx, "c1"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if wakes.Load() != 1 {
		t.Errorf("resident hit re-fired the wake (%d)", wakes.Load())
	}

	// Evict c1 (bound reached by touching another entity on a bound-1
	// store), then re-access: a genuine new wake with the interval since
	// c1's last activity (which the Update above refreshed).
	r2 := NewStore[custState]("cust", b, WithBound[custState](1),
		WithWake(func(_ imps.Entity, elapsed time.Duration, st custState) custState {
			wakes.Add(1)
			gotElapsed.Store(int64(elapsed))
			return st
		}))
	r2.now = clock.now
	if _, err := r2.Get(ctx, "c1"); err != nil { // rehydrate (wake #2 overall)
		t.Fatalf("get: %v", err)
	}
	clock.advance(2 * time.Minute)
	if _, err := r2.Get(ctx, "other"); err != nil { // evicts c1
		t.Fatalf("get other: %v", err)
	}
	if _, err := r2.Get(ctx, "c1"); err != nil { // re-wake
		t.Fatalf("re-get: %v", err)
	}
	// c1's last_active was refreshed by the earlier Update at +5m; now we
	// are at +7m, so this wake's elapsed is exactly 2 minutes... plus the
	// zero-advance between r2's first Get and the Update — measured from
	// the same stamp, which is the no-write-back contract.
	if time.Duration(gotElapsed.Load()) != 2*time.Minute {
		t.Errorf("re-wake elapsed = %v, want exactly 2m from the persisted stamp", time.Duration(gotElapsed.Load()))
	}
}

func TestWake_ConcurrentExactlyOnce(t *testing.T) {
	ctx := context.Background()
	b := newMemBackend()
	seed := NewStore[custState]("cust", b)
	if _, err := seed.Update(ctx, "c1", func(st custState) custState { st.Counter = 1; return st }); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var wakes atomic.Int64
	s := NewStore[custState]("cust", b,
		WithWake(func(_ imps.Entity, _ time.Duration, st custState) custState {
			wakes.Add(1)
			return st
		}))

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Get(ctx, "c1"); err != nil {
				t.Errorf("get: %v", err)
			}
		}()
	}
	wg.Wait()
	if wakes.Load() != 1 {
		t.Errorf("concurrent rehydration fired the wake %d times, want exactly 1", wakes.Load())
	}
}

func TestBound_HeldWithoutLossOrBackendTouches(t *testing.T) {
	ctx := context.Background()
	b := newMemBackend()
	const bound = 4
	const entities = 10
	s := NewStore[custState]("cust", b, WithBound[custState](bound))

	for i := 1; i <= entities; i++ {
		e := imps.Entity("e-" + strconv.Itoa(i))
		val := i * 10
		if _, err := s.Update(ctx, e, func(st custState) custState { st.Counter = val; return st }); err != nil {
			t.Fatalf("update %s: %v", e, err)
		}
		if r := s.Resident(); r > bound {
			t.Fatalf("resident = %d after %d entities, bound %d broken", r, i, bound)
		}
	}
	_, putsAfterWrites, deletesAfterWrites := b.counts()
	if putsAfterWrites != entities || deletesAfterWrites != 0 {
		t.Errorf("eviction touched the backend: puts=%d (want %d), deletes=%d (want 0)",
			putsAfterWrites, entities, deletesAfterWrites)
	}

	// Every entity reads back correct — evicted ones rehydrate.
	for i := 1; i <= entities; i++ {
		e := imps.Entity("e-" + strconv.Itoa(i))
		got, err := s.Get(ctx, e)
		if err != nil {
			t.Fatalf("get %s: %v", e, err)
		}
		if got.Counter != i*10 {
			t.Errorf("%s Counter = %d, want %d — lost across eviction", e, got.Counter, i*10)
		}
		if r := s.Resident(); r > bound {
			t.Fatalf("resident = %d during readback, bound broken", r)
		}
	}
	_, putsAfterReads, deletesAfterReads := b.counts()
	if putsAfterReads != putsAfterWrites || deletesAfterReads != 0 {
		t.Errorf("readback/eviction wrote to the backend: puts=%d, deletes=%d", putsAfterReads, deletesAfterReads)
	}
}
