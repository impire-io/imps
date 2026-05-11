package harness

import (
	"sync"
	"sync/atomic"
)

// shape holds the slot map and cap counter for one declared StateShape.
type shape struct {
	name    string
	cap     int
	factory func() any
	count   atomic.Int64
	slots   sync.Map // Entity → *slot
}

// slot is the per-entity record. A per-slot mutex serializes Update so two
// concurrent awareness/reasoning calls for the same entity each see a
// consistent snapshot.
type slot struct {
	mu  sync.Mutex
	val any
}

// registry holds every shape for an imp. It is constructed once at NewImp
// time and is concurrency-safe thereafter (the shape map is read-only; per-
// shape sync.Maps and atomics handle the rest).
type registry struct {
	shapes map[string]*shape
}

func newRegistry(shapes []StateShape) *registry {
	r := &registry{shapes: make(map[string]*shape, len(shapes))}
	for _, s := range shapes {
		r.shapes[s.Name] = &shape{
			name:    s.Name,
			cap:     s.Cap,
			factory: s.Factory,
		}
	}
	return r
}

// ref returns a StateRef for the (name, entity) slot. New-entity allocation
// is CAS-style: reserve a counter slot first, then store. If we lose the
// race (another goroutine populated the same entity), give the counter
// back. New-entity allocation past the shape's cap returns
// *ErrCapExceeded; reads/writes on existing slots succeed regardless of
// cap state.
func (r *registry) ref(name string, entity Entity) (StateRef, error) {
	sh, ok := r.shapes[name]
	if !ok {
		return nil, &ErrUnknownStateShape{Shape: name}
	}
	if existing, ok := sh.slots.Load(entity); ok {
		return &stateRef{slot: existing.(*slot)}, nil
	}
	for {
		current := sh.count.Load()
		if current >= int64(sh.cap) {
			return nil, &ErrCapExceeded{Shape: sh.name, Count: sh.cap}
		}
		if sh.count.CompareAndSwap(current, current+1) {
			break
		}
	}
	fresh := &slot{val: sh.factory()}
	if actual, loaded := sh.slots.LoadOrStore(entity, fresh); loaded {
		sh.count.Add(-1)
		return &stateRef{slot: actual.(*slot)}, nil
	}
	return &stateRef{slot: fresh}, nil
}

// stateRef is the concrete StateRef. Cross-shape ordering is not guaranteed
// (no global lock); per-slot serialization is sufficient for the per-entity
// "consistent snapshot" guarantee documented in data-model.md.
type stateRef struct {
	slot *slot
}

func (r *stateRef) Get() any {
	r.slot.mu.Lock()
	defer r.slot.mu.Unlock()
	return r.slot.val
}

func (r *stateRef) Set(v any) error {
	r.slot.mu.Lock()
	defer r.slot.mu.Unlock()
	r.slot.val = v
	return nil
}

func (r *stateRef) Update(fn func(any) any) error {
	r.slot.mu.Lock()
	defer r.slot.mu.Unlock()
	r.slot.val = fn(r.slot.val)
	return nil
}
