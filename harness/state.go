package harness

import "fmt"

// StateRef is a handle to a per-entity state slot. Get returns the value
// the factory produced (or the most recent Set). Set replaces the value.
// Update serializes a read-modify-write under the slot's lock; two
// awareness calls for the same entity each see a consistent snapshot.
type StateRef interface {
	Get() any
	Set(v any) error
	Update(fn func(any) any) error
}

// ErrUnknownStateShape is returned by AwarenessContext.State /
// ReasoningContext.State when the shape name is not declared on the
// ImpSpec.
type ErrUnknownStateShape struct {
	Shape string
}

func (e *ErrUnknownStateShape) Error() string {
	return fmt.Sprintf("harness: unknown state shape %q", e.Shape)
}

// ErrCapExceeded is returned by AwarenessContext.State /
// ReasoningContext.State when the requested entity is new and the shape's
// declared cap has been reached. No silent eviction occurs; reads/writes
// on existing slots continue to succeed.
type ErrCapExceeded struct {
	Shape string
	Count int
}

func (e *ErrCapExceeded) Error() string {
	return fmt.Sprintf("harness: state shape %q cap exceeded (%d)", e.Shape, e.Count)
}
