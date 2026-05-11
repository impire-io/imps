package harness

import (
	"errors"
	"strconv"
	"sync"
	"testing"
)

func newTestRegistry(name string, capacity int) *registry {
	return newRegistry([]StateShape{{
		Name:    name,
		Factory: func() any { return new(int) },
		Cap:     capacity,
	}})
}

func TestRegistry_AllocateAndReturnSameSlot(t *testing.T) {
	r := newTestRegistry("counter", 10)
	first, err := r.ref("counter", "e1")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Set(5); err != nil {
		t.Fatal(err)
	}
	second, err := r.ref("counter", "e1")
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Get().(int); got != 5 {
		t.Fatalf("expected stored value, got %v", got)
	}
}

func TestRegistry_CapExceededOnNewEntity(t *testing.T) {
	r := newTestRegistry("counter", 3)
	for i := 0; i < 3; i++ {
		if _, err := r.ref("counter", Entity(strconv.Itoa(i))); err != nil {
			t.Fatalf("alloc %d: %v", i, err)
		}
	}
	_, err := r.ref("counter", "overflow")
	var capErr *ErrCapExceeded
	if !errors.As(err, &capErr) || capErr.Shape != "counter" || capErr.Count != 3 {
		t.Fatalf("expected ErrCapExceeded{counter,3}, got %v", err)
	}
}

func TestRegistry_ReadsWritesAfterCap(t *testing.T) {
	r := newTestRegistry("counter", 1)
	ref, err := r.ref("counter", "e1")
	if err != nil {
		t.Fatal(err)
	}
	if err := ref.Set(42); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ref("counter", "e2"); err == nil {
		t.Fatal("expected cap exceeded")
	}
	again, err := r.ref("counter", "e1")
	if err != nil {
		t.Fatalf("existing entity should succeed: %v", err)
	}
	if got := again.Get().(int); got != 42 {
		t.Fatalf("expected 42 got %v", got)
	}
	if err := again.Update(func(v any) any { return v.(int) + 1 }); err != nil {
		t.Fatal(err)
	}
	if got := again.Get().(int); got != 43 {
		t.Fatalf("expected 43 got %v", got)
	}
}

func TestRegistry_UnknownShape(t *testing.T) {
	r := newTestRegistry("x", 1)
	_, err := r.ref("missing", "e1")
	var unk *ErrUnknownStateShape
	if !errors.As(err, &unk) || unk.Shape != "missing" {
		t.Fatalf("expected ErrUnknownStateShape{missing}, got %v", err)
	}
}

func TestRegistry_ConcurrentSameEntitySerialized(t *testing.T) {
	r := newTestRegistry("counter", 1)
	ref, _ := r.ref("counter", "e1")
	if err := ref.Set(0); err != nil {
		t.Fatal(err)
	}
	const goroutines = 50
	const each = 100
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r2, _ := r.ref("counter", "e1")
			for j := 0; j < each; j++ {
				_ = r2.Update(func(v any) any { return v.(int) + 1 })
			}
		}()
	}
	wg.Wait()
	final, _ := r.ref("counter", "e1")
	if got := final.Get().(int); got != goroutines*each {
		t.Fatalf("lost writes: got %d want %d", got, goroutines*each)
	}
}

func TestRegistry_ConcurrentNewEntitiesRespectCap(t *testing.T) {
	r := newTestRegistry("counter", 10)
	const total = 100
	var wg sync.WaitGroup
	var capErrs, ok int64
	var mu sync.Mutex
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := r.ref("counter", Entity(strconv.Itoa(i)))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else {
				var capErr *ErrCapExceeded
				if errors.As(err, &capErr) {
					capErrs++
				} else {
					t.Errorf("unexpected err: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
	if ok != 10 {
		t.Fatalf("expected 10 successful allocations, got %d (cap errs %d)", ok, capErrs)
	}
}
