// Package persist is the durable tier of an imp's two-tier local memory:
// a bounded, write-through, rehydrate-on-access store for per-entity state,
// plus the imp-level sleep clock (the Beacon).
//
// # The two-tier rule
//
// ImpSpec.States (the harness registry) is the ephemeral hot tier: state
// that can be rebuilt from the stream. This package is the durable tier:
// state whose loss on a restart would be a bug. One tier per concern — the
// package never synchronizes the two, and the harness registry is untouched
// by it.
//
// # Durability and restore
//
// The store is write-through: Update returns success only after the backend
// accepted the entity's envelope (codec-encoded state + last-active stamp),
// so the snapshot is continuous and stopping the imp is always safe — there
// is no flush step whose omission can lose data. Restore is lazy: nothing
// loads at startup; an entity rehydrates on first access. A never-seen
// entity yields the zero state with no error; a backend or decode failure
// yields an error, never a silent zero.
//
// # The wake contract
//
// A WakeFn advances time-dependent state (decay, "idle since", debounce) by
// the elapsed interval since the entity's last activity. It fires exactly
// once per rehydration, before the state is observable to the caller; never
// on resident hits or never-seen entities; and it never writes back — so a
// wake after eviction recomputes from the same last-active stamp. Write
// wake functions as pure "advance to now" transformations and re-firing is
// harmless by construction.
//
// # Eviction and removal
//
// Residency is LRU-bounded (default 256; "the default stays small").
// Eviction is a pure drop — write-through guarantees the backend already
// holds the latest state, so nothing is lost and the backend is never
// written or deleted by eviction. Delete is the only operation that removes
// backend state.
//
// # Use from awareness and thinking
//
// Awareness may call Get or Update — each is a single bounded round-trip,
// the same discipline as AwarenessContext.Request. At most one or two store
// calls per dispatch; anything heavier belongs in thinking. Never loop over
// entities in awareness.
//
// # The Beacon — the restart clock
//
// The whole-imp elapsed reading across stops, deploys, and
// (heartbeat-bounded) crashes: Stamp liveness on a heartbeat and at
// shutdown; at startup ask SleptFor and run the imp-level wake step before
// imp.Run — "a single call, before any channel dispatch resumes". A
// first-ever start reports absence (ok=false), not a zero sleep. The Beacon
// is a self-report, not the sleep signal: snapshot-suspension of a running
// imp is the runtime's act, only the suspender knows that interval, and a
// resume continues mid-Run where no gate re-executes — that wake path is
// deliberately unbuilt until it is co-designed with the runtime (see
// hq/02-DESIGN/0004-sleep-wake-persistence.md, "out of scope — M2b").
package persist
