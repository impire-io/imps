package persist

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	imps "github.com/impire-io/imps"
)

// envelope is the unit of persistence: the codec's output carried as opaque
// bytes, plus the entity's last-active wall-clock stamp (the wake hook's
// elapsed source). One JSON value per entity — state and stamp can never
// tear apart.
type envelope struct {
	State      []byte    `json:"state"`
	LastActive time.Time `json:"last_active"`
}

// WakeFn advances time-dependent state by the elapsed interval since the
// entity's last activity. It runs exactly once per rehydration, before the
// state is observable, and never writes back — express it as a pure
// "advance to now" transformation so a re-fire after eviction (computed
// from the same last-active stamp) is harmless by construction.
type WakeFn[T any] func(entity imps.Entity, elapsed time.Duration, state T) T

// DefaultBound is the resident bound when WithBound is not given. The
// default is small and stays small.
const DefaultBound = 256

// Option customises a Store.
type Option[T any] func(*config[T])

type config[T any] struct {
	bound int
	wake  WakeFn[T]
	codec Codec[T]
}

// WithBound sets the resident bound (LRU). n must be > 0.
func WithBound[T any](n int) Option[T] {
	if n <= 0 {
		panic("persist: WithBound requires n > 0")
	}
	return func(c *config[T]) { c.bound = n }
}

// WithWake sets the per-entity wake hook.
func WithWake[T any](fn WakeFn[T]) Option[T] {
	return func(c *config[T]) { c.wake = fn }
}

// WithCodec replaces the default JSONCodec.
func WithCodec[T any](codec Codec[T]) Option[T] {
	return func(c *config[T]) { c.codec = codec }
}

// Store is the durable tier for one named kind of per-entity state:
// bounded residency, write-through persistence, rehydration on access.
// Operations are concurrency-safe and serialize internally.
type Store[T any] struct {
	name  string
	b     Backend
	bound int
	wake  WakeFn[T]
	codec Codec[T]
	now   func() time.Time // injectable from same-package tests

	mu       sync.Mutex
	resident map[imps.Entity]*list.Element
	lru      *list.List // front = hottest, back = coldest
}

type entry[T any] struct {
	entity imps.Entity
	state  T
}

// NewStore builds the store. name scopes backend keys as "<name>.<entity>"
// and must be non-empty; several stores share a backend namespace without
// collision as long as their names differ.
func NewStore[T any](name string, b Backend, opts ...Option[T]) *Store[T] {
	if name == "" {
		panic("persist: store name required")
	}
	cfg := config[T]{bound: DefaultBound, codec: JSONCodec[T]()}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Store[T]{
		name:     name,
		b:        b,
		bound:    cfg.bound,
		wake:     cfg.wake,
		codec:    cfg.codec,
		now:      time.Now,
		resident: make(map[imps.Entity]*list.Element),
		lru:      list.New(),
	}
}

func (s *Store[T]) key(entity imps.Entity) string {
	return s.name + "." + string(entity)
}

// Get returns the entity's state: resident hit (no wake), rehydration from
// the backend (wake fires first), or the zero value for a never-seen entity
// (no wake, no error). Backend and decode failures return an error — never
// a silent zero.
func (s *Store[T]) Get(ctx context.Context, entity imps.Entity) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(ctx, entity)
}

// getLocked is Get under s.mu — shared with Update so the wake runs before
// the caller's fn sees the state.
func (s *Store[T]) getLocked(ctx context.Context, entity imps.Entity) (T, error) {
	if el, ok := s.resident[entity]; ok {
		s.lru.MoveToFront(el)
		return el.Value.(*entry[T]).state, nil
	}
	var state T
	raw, err := s.b.Get(ctx, s.key(entity))
	switch {
	case errors.Is(err, ErrNotFound):
		// Never-seen entity: zero state, no wake — nothing to advance.
	case err != nil:
		return state, err
	default:
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return state, fmt.Errorf("persist: decode envelope %s: %w", entity, err)
		}
		if state, err = s.codec.Unmarshal(env.State); err != nil {
			return state, fmt.Errorf("persist: decode state %s: %w", entity, err)
		}
		if s.wake != nil {
			state = s.wake(entity, s.now().Sub(env.LastActive), state)
		}
	}
	s.insert(entity, state)
	return state, nil
}

// Update runs a read-modify-write with write-through: any due wake runs
// before fn sees the state, and the new state plus a fresh last-active
// stamp are durable on the backend before Update returns success. On a
// backend failure the resident state is left at its pre-fn value.
func (s *Store[T]) Update(ctx context.Context, entity imps.Entity, fn func(T) T) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.getLocked(ctx, entity)
	if err != nil {
		return cur, err
	}
	next := fn(cur)
	stateRaw, err := s.codec.Marshal(next)
	if err != nil {
		return next, fmt.Errorf("persist: encode state %s: %w", entity, err)
	}
	envRaw, err := json.Marshal(envelope{State: stateRaw, LastActive: s.now()})
	if err != nil {
		return next, fmt.Errorf("persist: encode envelope %s: %w", entity, err)
	}
	if err := s.b.Put(ctx, s.key(entity), envRaw); err != nil {
		return next, err
	}
	// Durable — now (and only now) reflect it in residency.
	if el, ok := s.resident[entity]; ok {
		el.Value.(*entry[T]).state = next
		s.lru.MoveToFront(el)
	} else {
		s.insert(entity, next)
	}
	return next, nil
}

// Delete removes the entity from the backend and from residency — the only
// operation that ever removes backend state. Deleting an unknown entity is
// not an error. On a backend failure residency is left intact (retryable).
func (s *Store[T]) Delete(ctx context.Context, entity imps.Entity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.b.Delete(ctx, s.key(entity)); err != nil {
		return err
	}
	if el, ok := s.resident[entity]; ok {
		s.lru.Remove(el)
		delete(s.resident, entity)
	}
	return nil
}

// Resident reports the current resident count.
func (s *Store[T]) Resident() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lru.Len()
}

// insert adds a resident entry, evicting the coldest first at the bound.
// Eviction is a pure drop: write-through guarantees the backend already
// holds the evicted entity's latest state, and eviction never writes or
// deletes. Callers hold s.mu.
func (s *Store[T]) insert(entity imps.Entity, state T) {
	for s.lru.Len() >= s.bound {
		coldest := s.lru.Back()
		s.lru.Remove(coldest)
		delete(s.resident, coldest.Value.(*entry[T]).entity)
	}
	s.resident[entity] = s.lru.PushFront(&entry[T]{entity: entity, state: state})
}
