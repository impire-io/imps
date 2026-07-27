# Per-Entity Persistence and the Restart Clock

How an imp's per-entity memory survives stops, deploys, and crashes:
eviction under a bound, rehydration on access, a snapshot/restore contract,
the per-entity rehydration wake, and the imp-level restart clock. This
document is the **M2a** design (roadmap; graduated from the
`sleep-wake-persistence` research topic,
[episode 0005](../04-JOURNEY/0005-sleep-wake-persistence.md), and re-scoped
by the boundary verdict of
[episode 0007](../04-JOURNEY/0007-sleep-boundary-with-soulrealm.md)).
Everything here is **[V]** — shipped as feature `005-sleep-wake-persistence`
([episode 0006](../04-JOURNEY/0006-sleep-wake-persistence-shipped.md)); the
implementation matched this contract with no drift (one addition: the
default bound is the exported constant `DefaultBound = 256`).

**Deliberately out of scope — M2b:** whole-imp snapshot sleep and its
authoritative wake. Suspending and resuming a running imp is the runtime's
act (in the Impire family, the soulrealm runtime and its isolation
backends), and only the suspender can supply an authoritative slept-for —
no imp code runs at suspend time. The imps side of that contract (an
elapsed reading delivered *mid-process* before dispatch resumes) is `[D]`,
gated on soulrealm growing suspend/resume and a co-designed wake-delivery
contract (episode 0007). The `Beacon` below is the **restart clock** — the
interim, self-reported imp-level elapsed source — not the sleep signal.

The load-bearing finding this design rests on `[measured]`: persistence
lives **beside** the registry, not inside it. Riding the shipped registry is
blocked four ways at once (documented cap-rejection with "no silent
eviction", an error-less `StateRef.Get`, an entity-less `Factory`, no
enumeration), but the boundary needs none of it — the spike ran a full
restart round-trip, exactly-once wake with true elapsed time, and lossless
bounded eviction with **zero harness changes**, against a reference backend.
Per the constitution's "boundaries before mechanisms", this document commits
to no backend: it specifies the boundary, the envelope, and a reference
implementation.

## Where it lives: a package in the core module, beside the harness

Persistence ships as **`imps/persist`** — a plain package in the existing
module (like `testutil`), NOT a nested module. M1's nested-module shape
existed to fence a new dependency; `persist` adds **none** (its reference
backend uses `nats.go/jetstream`, already a core dependency), so the module
boundary would be ceremony. The constraints that matter survive unchanged:

- The root package (the harness) is not modified: no registry change, no
  context change, no new verdict. `ImpSpec.States` remains what it is — the
  **ephemeral hot tier**, in-memory, cap-rejecting, never evicting.
- The core `go.mod` gains nothing.
- Importing `imps/persist` is opt-in; an imp that doesn't persist links
  nothing new.

## The two-tier memory boundary

| Tier | Surface | Holds | Lifetime |
|---|---|---|---|
| Ephemeral (shipped) | `ImpSpec.States` → `State(name, entity)` | scratch interpretation state that can be rebuilt from the stream | process |
| Durable (this design) | `persist.Store[T]` | per-entity state that must survive sleep and restarts | backend |

One entity's state lives in **one** tier per concern — the design's rule of
thumb, documented in the package: if losing it on restart is a bug, it goes
in the store; if it's rebuildable, the registry. The registered reversal
condition for this split: real imps showing two-tier inconsistency bugs, or
measured dispatch-latency damage from bounded IO in awareness, moves the
boundary into the harness as its own redesigned feature.

## The snapshot/restore contract

One envelope per entity, written **through** on every update — the snapshot
is continuous; there is no schedule to tune and nothing to flush at
shutdown:

```json
{ "state":       <codec-encoded state bytes>,
  "last_active": "<RFC3339Nano wall-clock stamp>" }
```

- Key: `<prefix>.<entity>` in the backend's keyspace (prefix = the store's
  name, so several stores share one bucket without collision).
- `Update` returns only after the backend accepted the envelope — durability
  is the return-value contract.
- Restore is lazy: nothing is loaded at startup; an entity rehydrates on its
  first access (`Get`/`Update`), which is when the wake hook fires.
- Cold-start **message** replay is not this contract's job: feature 004's
  durable consumers already position channels across restarts (anatomy,
  "consumer position across sleep/wake cycles").

## Surfaces

```go
package persist // import "github.com/impire-io/imps/persist"

// Backend is the minimal persistence boundary. The framework commits to no
// implementation; KVBackend is the reference.
type Backend interface {
    Get(ctx context.Context, key string) ([]byte, error) // ErrNotFound on miss
    Put(ctx context.Context, key string, value []byte) error
    Delete(ctx context.Context, key string) error
}
var ErrNotFound = errors.New("persist: not found")

// KVBackend adapts a JetStream key-value bucket (the reference backend).
func KVBackend(kv jetstream.KeyValue) Backend

// WakeFn advances time-dependent state by the elapsed interval. Called
// exactly once per rehydration, before the state is returned to the caller.
type WakeFn[T any] func(entity imps.Entity, elapsed time.Duration, state T) T

// NewStore builds a bounded, write-through, rehydrate-on-access store.
// name scopes the keys; the codec defaults to JSON; the bound defaults
// small (256 resident entities).
func NewStore[T any](name string, b Backend, opts ...Option[T]) *Store[T]

func WithBound[T any](n int) Option[T]            // resident bound (LRU)
func WithWake[T any](fn WakeFn[T]) Option[T]      // per-entity wake hook
func WithCodec[T any](c Codec[T]) Option[T]       // replaces JSON
type Codec[T any] interface {
    Marshal(T) ([]byte, error)
    Unmarshal([]byte) (T, error)
}

// Get: resident hit; else rehydrate from the backend (wake fires); else the
// zero value for a never-seen entity (no wake — nothing to advance).
func (s *Store[T]) Get(ctx context.Context, entity imps.Entity) (T, error)

// Update: read-modify-write with write-through; the wake (if due) runs
// before fn sees the state; the fresh last-active stamp lands with the new
// state before Update returns.
func (s *Store[T]) Update(ctx context.Context, entity imps.Entity, fn func(T) T) (T, error)

// Delete removes the entity from residency AND the backend — the only way
// backend state is ever removed. Eviction never deletes.
func (s *Store[T]) Delete(ctx context.Context, entity imps.Entity) error

// Resident reports the current resident count (observability, tests).
func (s *Store[T]) Resident() int

// Beacon is the imp-level sleep clock: an imp-scoped last-active stamp in
// the same backend.
func NewBeacon(name string, b Backend) *Beacon
func (b *Beacon) Stamp(ctx context.Context) error                  // call on a heartbeat and at shutdown
func (b *Beacon) SleptFor(ctx context.Context) (time.Duration, bool, error) // false on first ever start
```

### Eviction and residency

- The resident set is an LRU bounded by `WithBound` (default 256 — "the
  default stays small"). At the bound, the coldest entity is **dropped** —
  write-through guarantees the backend already holds its latest state, so
  eviction is lossless by construction `[measured]`.
- Eviction MUST NOT write, flush, or delete. A later access rehydrates.
- Exceeding the bound never rejects work (unlike the registry's
  cap-rejection): new entities always get residency by evicting the coldest.

### Wake-hook semantics

- **Per-entity** (this store): the wake fires **exactly once per
  rehydration**, with `elapsed = now − envelope.last_active`, *before* the
  state is visible to the caller. Resident hits never fire it; never-seen
  entities never fire it. Eviction followed by re-access fires it again —
  that access IS a new wake of that entity, and the interval is genuinely
  the time since it was last active `[measured]`.
- **Imp-level** (the Beacon — the restart clock): the anatomy's contract —
  "a single call, before any channel dispatch resumes" — is a gate in
  `main()`: ask `SleptFor`, run the imp's wake hook, then `Run`. This covers
  graceful stops, deploys, and (heartbeat-bounded) crashes. It does **not**
  cover snapshot-suspension — a resume continues mid-`Run` and the gate
  never re-executes; that path is M2b's, co-designed with the runtime
  (episode 0007).

### Use from the two tiers

- Awareness MAY call `Get`/`Update` — each is a single bounded round-trip,
  the same discipline as awareness's own `Request` and M1's note bridge
  `[mechanism-argument]`. At most one or two store calls per dispatch;
  anything heavier escalates to thinking.
- Thinking uses the store freely.
- The store is constructed in `main()` and captured by the closures that
  need it — exactly the M1 `Participant` pattern. Handing it around is the
  imp author's act; the package documentation forbids unbounded loops over
  entities in awareness.

## Boundaries (MUST / MUST NOT)

- The harness core MUST NOT change: no registry, context, dispatch, or spec
  surface edits; `compile-deny` invariants untouched. (Measured attainable:
  the spike needed nothing.)
- `Update` MUST be write-through — returning success before the backend
  accepted the envelope is a correctness bug, not an optimization.
- The wake hook MUST run before the rehydrated state is observable, and
  MUST NOT run on resident hits or never-seen entities.
- Eviction MUST NOT touch the backend; only `Delete` removes state.
- The package MUST NOT enumerate, scan, or migrate backend contents — no
  registry-of-entities, no janitor. (Operators own their buckets.)
- The framework MUST NOT commit to a backend: `Backend` is the boundary,
  `KVBackend` the reference implementation, and nothing in the harness or
  this package requires NATS KV specifically.

## Acceptance criteria

1. A real imp mutating store state from awareness, stopped and replaced by a
   fresh instance, rehydrates every touched entity **equal under the codec**
   to its pre-stop value (the research spike, productized).
2. The per-entity wake fires exactly once per rehydration with an elapsed
   ≥ the true stop duration and bounded by wall clock; the state advance is
   a pure function of the delivered elapsed; resident re-access does not
   re-fire.
3. N > bound entities: residency never exceeds the bound during writes or
   readback, and zero state is lost across evictions.
4. `Beacon.SleptFor` across an imp stop measures the stop duration within
   wall-clock tolerance; first-ever start reports absence, not zero.
5. A backend failure surfaces as an error from `Get`/`Update` (never a
   silent zero state); a never-seen entity yields the zero value with no
   wake and no error.
6. The root package and core `go.mod` are byte-identical before and after
   the feature; the full gate (both modules) is green with the new package
   covered; `compile-deny` green.

## Decisions and tradeoffs

- **Beside the registry, not inside it.** The adversarial pass (episode
  0005) refuted the harness-native route on measured grounds: it breaks four
  documented registry guarantees and moves backend IO into the dispatch
  path. The registry stays the ephemeral tier; reversal condition registered
  above.
- **A package, not a nested module** — refined from the research's
  "glue-module" resolution at design time: M1's module boundary fenced a new
  dependency; `persist` adds none, so the simpler shape wins.
- **Write-through over scheduled snapshots.** Continuous durability, nothing
  to flush, eviction lossless by construction; the cost is one backend
  round-trip per update. The named upgrade path if update-rate evidence
  appears: write-back batching inside the package — never a harness change.
- **Lazy restore over startup replay.** Rehydrate-on-access keeps cold
  starts fast and memory small; an imp that wants warm-up can touch its hot
  entities in `main()`.
- **The Beacon is explicit, not automatic — and interim.** The harness does
  not know it stopped; a stamped clock in the backend is the honest
  self-reported source, and the pre-`Run` gate satisfies "before any channel
  dispatch resumes" without a harness hook. When the runtime grows
  suspend/resume (M2b), its authoritative signal supersedes the Beacon for
  the sleep path; the Beacon remains the restart clock.
