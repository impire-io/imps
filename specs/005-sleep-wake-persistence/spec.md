# Feature Specification: Sleep, Wake, and Per-Entity Persistence

**Feature Branch**: `005-sleep-wake-persistence`
**Created**: 2026-07-26
**Status**: Shipped
**Input**: User description: "Sleep, wake, and per-entity persistence per hq/02-DESIGN/0004-sleep-wake-persistence.md: an imps/persist package in the core module providing a bounded, write-through, rehydrate-on-access per-entity store, wake hooks at entity and imp level, and a backend-agnostic boundary."

**Design source**: [`hq/02-DESIGN/0004-sleep-wake-persistence.md`](../../hq/02-DESIGN/0004-sleep-wake-persistence.md)
(graduated from research, [episode 0005](../../hq/04-JOURNEY/0005-sleep-wake-persistence.md)).
The framework commits to no persistence backend; the boundary is the
contract, with one reference implementation.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - State survives a restart (Priority: P1)

An imp developer keeps per-entity state in a durable store instead of (or
beside) the ephemeral in-memory tier. Every update is durable the moment it
returns. When the imp stops — sleep, deploy, crash — and a fresh instance
starts, each entity's state comes back on first access exactly as it was,
with no startup replay step and no code beyond declaring the store.

**Why this priority**: durability across restarts is the entire reason M2
exists; an imp whose interpretation evaporates on every deploy cannot hold a
long-running job.

**Independent Test**: run an imp that mutates an entity's state through the
store, stop it, start a fresh instance against the same backend, and verify
the first access returns state equal (under the configured codec) to the
pre-stop value.

**Acceptance Scenarios**:

1. **Given** an imp that updated an entity's state through the store,
   **When** the imp stops and a fresh instance accesses that entity,
   **Then** the returned state equals the pre-stop state under the codec.
2. **Given** a store update in progress, **When** the update call returns
   success, **Then** the state is already durable on the backend — there is
   no flush step at shutdown whose omission can lose data.
3. **Given** an entity never seen before, **When** it is first accessed,
   **Then** the caller receives the zero state with no error and no wake
   signal (there is nothing to advance).
4. **Given** a backend failure, **When** an access or update hits it,
   **Then** the call returns an error — never a silent zero state.

---

### User Story 2 - Time-dependent state wakes correctly (Priority: P2)

Some state decays or ages with wall-clock time (moving averages, "idle
since", debounce windows). When an entity's state is rehydrated after any
gap — an imp restart, or eviction and re-access — the developer's wake hook
receives the true elapsed time since the entity's last activity and advances
the state before any other code sees it.

**Why this priority**: the anatomy calls time-skip after sleep a real bug
class — without the wake hook, time-dependent state silently produces wrong
answers after every sleep.

**Independent Test**: stop an imp for a measured interval; on rehydration,
verify the hook fired exactly once, received an elapsed value at least the
stop duration and bounded by wall clock, and that the state visible to the
caller reflects the hook's advancement.

**Acceptance Scenarios**:

1. **Given** an entity persisted before a stop of measured length, **When**
   a fresh instance rehydrates it, **Then** the wake hook fires exactly
   once with elapsed ≥ the stop duration, before the state is observable.
2. **Given** a rehydrated entity resident in memory, **When** it is
   accessed again, **Then** the wake hook does not fire again.
3. **Given** an entity evicted and later re-accessed within one process,
   **When** it rehydrates, **Then** the wake hook fires for that new wake
   with the genuine interval since the entity's last activity.
4. **Given** an imp that slept as a whole (process suspended or stopped),
   **When** it starts, **Then** the developer can ask how long the imp
   slept and run an imp-level wake step before any channel dispatch begins;
   a first-ever start reports "never stamped", not a zero duration.

---

### User Story 3 - Memory stays bounded without losing state (Priority: P3)

An imp tracks more entities over its lifetime than it can hold in memory.
The store keeps only a bounded resident set, silently dropping the coldest
entities — and because every update was already durable, a dropped entity's
state is simply reloaded on its next access. New entities always get room;
nothing is ever rejected and nothing is ever lost.

**Why this priority**: "the default is small and stays small" is the
anatomy's memory commitment; unbounded residency is a slow leak in any
long-running imp.

**Independent Test**: touch more entities than the configured bound; verify
residency never exceeds the bound, every entity's state reads back correct
afterward, and no access was ever rejected.

**Acceptance Scenarios**:

1. **Given** a bound of N, **When** more than N entities are touched,
   **Then** the resident count never exceeds N and every touch succeeds.
2. **Given** an entity evicted from residency, **When** it is accessed
   again, **Then** its state is rehydrated intact from the backend.
3. **Given** any eviction, **When** it happens, **Then** the backend is not
   written, flushed, or deleted — eviction is a pure drop.
4. **Given** a developer who removes an entity deliberately, **When** they
   call the explicit delete, **Then** the entity is gone from both
   residency and the backend — and nothing else ever removes backend state.

---

### Edge Cases

- **Both tiers in use**: the ephemeral tier (`ImpSpec.States`) and the
  durable store coexist; the documented rule of thumb is one tier per
  concern (rebuildable → ephemeral; loss-is-a-bug → durable). The package
  never synchronizes the two.
- **Wake without write-back**: the wake hook advances state in memory only;
  the envelope on the backend keeps its stamp until the next update. A
  re-wake after eviction therefore recomputes from the same last-activity
  point — wake functions must be expressed as "advance to now", which makes
  re-firing harmless by construction.
- **Concurrent access**: store operations serialize; the wake-exactly-once
  guarantee holds under concurrent Get/Update for the same entity.
- **Malformed envelope on the backend**: surfaces as an error from the
  access — never a silent zero state.
- **Awareness usage**: at most one or two bounded store calls per dispatch
  (the same discipline as the framework's own bounded request); heavier
  state work belongs to thinking. This is documentation-enforced, like the
  M1 note bridge.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The feature MUST ship as a new package inside the existing
  core module (`imps/persist`), with the root harness package untouched and
  the core module's dependency manifest byte-identical — the feature adds
  no dependencies.
- **FR-002**: The store MUST be write-through: an update returns success
  only after the backend has accepted the entity's envelope. There MUST be
  no flush-at-shutdown step.
- **FR-003**: The envelope MUST carry the codec-encoded state and the
  entity's last-active wall-clock stamp, keyed `<store-name>.<entity>`, so
  several stores can share one backend namespace without collision.
- **FR-004**: Restore MUST be lazy: nothing loads at startup; an entity
  rehydrates on first access. A never-seen entity yields the zero state
  with no wake and no error; a backend or decode failure yields an error,
  never a silent zero.
- **FR-005**: The per-entity wake hook MUST fire exactly once per
  rehydration, before the state is observable to the caller, with
  elapsed = now − last-active. It MUST NOT fire on resident hits or
  never-seen entities. It MUST NOT write to the backend.
- **FR-006**: Residency MUST be bounded (small default, 256) with
  least-recently-used eviction; eviction MUST be a pure drop (no backend
  write or delete) and MUST never reject a new entity.
- **FR-007**: Explicit delete MUST be the only operation that removes
  backend state, and MUST clear residency too.
- **FR-008**: The backend boundary MUST be a minimal interface (get / put /
  delete with a distinguishable not-found), with a reference implementation
  over the substrate's key-value facility. Nothing in the framework may
  require that specific backend.
- **FR-009**: An imp-level sleep clock (the Beacon) MUST let the developer
  stamp liveness and, at startup, read how long the imp slept — with a
  first-ever start distinguishable from a zero-length sleep — so an
  imp-level wake step can run before channel dispatch starts.
- **FR-010**: The codec MUST default to JSON and be replaceable per store;
  state equality across a restart is defined under the configured codec.
- **FR-011**: Store operations MUST be safe under concurrent use, and the
  wake-exactly-once guarantee MUST hold under concurrency.
- **FR-012**: The package MUST NOT enumerate, scan, or migrate backend
  contents, MUST NOT synchronize the two memory tiers, and MUST carry no
  janitor duties.

### Key Entities

- **Envelope**: the unit of persistence — one entity's codec-encoded state
  plus its last-active stamp; the snapshot/restore contract.
- **Store**: the bounded, write-through, rehydrate-on-access holder of one
  named kind of per-entity state; the durable tier of the two-tier memory
  boundary.
- **Wake hook (per-entity)**: the developer's advance-by-elapsed function,
  run exactly once per rehydration before the state is visible.
- **Beacon**: the imp-scoped sleep clock — stamp on a heartbeat/shutdown,
  read the slept interval before dispatch starts.
- **Backend**: the minimal persistence boundary the framework refuses to
  commit beyond; one reference implementation ships.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of entities updated before a stop read back equal under
  the codec after a fresh instance starts, with zero startup replay work.
- **SC-002**: Across restart, eviction/re-access, and concurrent access,
  the wake hook fires exactly once per rehydration, its elapsed is ≥ the
  true gap and wall-clock-bounded, and no state is observable pre-wake.
- **SC-003**: With N entities > bound, residency never exceeds the bound,
  zero accesses are rejected, and zero state is lost.
- **SC-004**: The imp-level sleep reading across a measured stop is ≥ the
  stop duration and wall-clock-bounded; a first-ever start is reported as
  such, not as a zero sleep.
- **SC-005**: The harness core is provably untouched: root package
  unmodified, dependency manifest byte-identical, boundary invariants
  (compile-deny) green.
- **SC-006**: The full repository gate passes with the new package covered
  and no test skipped; backend failures in tests surface as errors in 100%
  of cases (no silent zeros).

## Assumptions

- The backend namespace (e.g. the key-value bucket) is provisioned by the
  operator; the package never provisions.
- One store per state kind, named uniquely per imp deployment; key-space
  discipline across imps sharing a bucket is the operator's concern.
- Wake functions are pure in the elapsed interval ("advance to now"), which
  the no-write-back re-fire semantics rely on; this is documented as the
  contract of the hook.
- Notes-rate-style pressure on store IO (one backend round-trip per update)
  is acceptable at imp scale; write-back batching inside the package is the
  named upgrade path if update-rate evidence appears.
- The isolation mechanism for whole-imp sleep (microVM, container, process
  stop) remains an infrastructure choice; the Beacon only supplies the
  elapsed reading.
- Runtime behavior under the reversal conditions registered in episode 0005
  (two-tier inconsistency bugs; measured dispatch-latency damage) would
  reopen placement — out of scope here.
