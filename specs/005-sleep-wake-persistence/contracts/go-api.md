# Contract: Exported Go API of `github.com/impire-io/imps/persist`

The complete exported surface. Anything not listed is unexported or does not
ship in this feature. Signatures are contractual; the semantics below each
item are part of the contract.

```go
package persist // import "github.com/impire-io/imps/persist"
```

## The backend boundary

```go
// ErrNotFound reports a key absent from the backend. errors.Is-able.
var ErrNotFound = errors.New("persist: not found")

// Backend is the minimal persistence boundary. The framework commits to no
// implementation; KVBackend is the reference.
type Backend interface {
    Get(ctx context.Context, key string) ([]byte, error) // ErrNotFound on miss
    Put(ctx context.Context, key string, value []byte) error
    Delete(ctx context.Context, key string) error // absent key is not an error
}

// KVBackend adapts a JetStream key-value bucket (the reference backend).
// The bucket is provisioned by the operator; the adapter never creates it.
func KVBackend(kv jetstream.KeyValue) Backend
```

## Codec

```go
// Codec encodes and decodes one state kind. The default is JSONCodec.
type Codec[T any] interface {
    Marshal(T) ([]byte, error)
    Unmarshal([]byte) (T, error)
}

// JSONCodec is the default codec (encoding/json).
func JSONCodec[T any]() Codec[T]
```

## The store

```go
// WakeFn advances time-dependent state by the elapsed interval. It MUST be
// pure in elapsed ("advance to now"): it runs exactly once per rehydration,
// never writes back, and a re-fire after eviction recomputes from the same
// last-active stamp.
type WakeFn[T any] func(entity imps.Entity, elapsed time.Duration, state T) T

// NewStore builds the durable tier for one named state kind: bounded
// residency (LRU, default 256), write-through persistence, rehydration on
// access. name scopes backend keys as "<name>.<entity>".
func NewStore[T any](name string, b Backend, opts ...Option[T]) *Store[T]

type Option[T any] func(*config[T])

func WithBound[T any](n int) Option[T]       // resident bound; n must be > 0
func WithWake[T any](fn WakeFn[T]) Option[T] // per-entity wake hook
func WithCodec[T any](c Codec[T]) Option[T]  // replaces JSONCodec

// Get returns the entity's state: resident hit (no wake), or rehydration
// from the backend (wake fires first, before the state is observable), or
// the zero value for a never-seen entity (no wake, no error). Backend and
// decode failures return an error — never a silent zero.
func (s *Store[T]) Get(ctx context.Context, entity imps.Entity) (T, error)

// Update is a read-modify-write with write-through: any due wake runs
// before fn sees the state; the new state and a fresh last-active stamp are
// durable on the backend before Update returns success.
func (s *Store[T]) Update(ctx context.Context, entity imps.Entity, fn func(T) T) (T, error)

// Delete removes the entity from residency AND the backend — the only
// operation that ever removes backend state. Deleting an unknown entity is
// not an error.
func (s *Store[T]) Delete(ctx context.Context, entity imps.Entity) error

// Resident reports the current resident count (observability, tests).
func (s *Store[T]) Resident() int
```

Contractual behavior:

- Write-through: `Update` success ⇒ envelope durable. No flush step exists.
- Eviction: pure drop at the bound (coldest first); never writes, never
  deletes, never rejects a new entity.
- Wake: exactly once per rehydration, pre-visibility; never on resident
  hits or never-seen entities; holds under concurrent access (`-race`).
- Operations are concurrency-safe; the store serializes internally.
- The store never enumerates or scans the backend.

## The beacon

```go
// Beacon is the imp-level sleep clock: an imp-scoped last-active stamp
// under its own backend key ("<name>").
func NewBeacon(name string, b Backend) *Beacon

// Stamp records liveness now. Call on a heartbeat and at shutdown.
func (b *Beacon) Stamp(ctx context.Context) error

// SleptFor reads the elapsed interval since the last stamp. ok=false means
// never stamped (a first-ever start) — distinguishable from a zero sleep.
func (b *Beacon) SleptFor(ctx context.Context) (elapsed time.Duration, ok bool, err error)
```

Documented usage pattern (the pre-`Run` gate, not enforced by the harness):

```go
slept, ok, err := beacon.SleptFor(ctx)
if err != nil { ... }
if ok { advanceImpLevelState(slept) }
// then, and only then:
imp.Run(ctx)
```

## Package documentation contract

`doc.go` MUST state: the two-tier memory rule (registry = ephemeral,
rebuildable; store = durable, loss-is-a-bug) and that the package never
synchronizes the tiers; the wake contract (exactly-once per rehydration,
pre-visibility, advance-to-now purity, no write-back); the awareness
discipline (at most bounded Get/Update per dispatch — the Request
discipline); and that eviction never touches the backend while Delete is
the only removal.
