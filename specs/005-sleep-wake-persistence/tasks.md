# Tasks: Sleep, Wake, and Per-Entity Persistence

**Input**: Design documents from `/specs/005-sleep-wake-persistence/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/go-api.md, contracts/repo-gate.md, quickstart.md

**Tests**: included — the success criteria are measured readings and the research method requires the spike to become the permanent suite (research.md D8).

**Organization**: tasks grouped by user story. All code lives in the NEW `persist/` package of the core module; the root harness package, `go.mod`, `Makefile`, and CI are untouched by every task (FR-001, contracts/repo-gate.md).

## Phase 1: Setup

- [x] T001 Write `persist/doc.go` package documentation per the `contracts/go-api.md` documentation contract: the two-tier memory rule (registry = ephemeral/rebuildable, store = durable/loss-is-a-bug, never synchronized), the wake contract (exactly-once per rehydration, pre-visibility, advance-to-now purity, no write-back), the awareness discipline (at most bounded Get/Update per dispatch), and eviction-never-deletes / Delete-is-the-only-removal

## Phase 2: Foundational (blocking prerequisites for all user stories)

- [x] T002 Implement `persist/backend.go`: `ErrNotFound` sentinel, the `Backend` interface (Get/Put/Delete; Delete of an absent key is not an error), and `KVBackend(kv jetstream.KeyValue)` mapping the substrate's not-found to `ErrNotFound`, per `contracts/go-api.md`
- [x] T003 [P] Implement `persist/codec.go`: `Codec[T]` and `JSONCodec[T]()` (encoding/json), per `contracts/go-api.md`
- [x] T004 [P] Unit tests in `persist/backend_test.go`: `KVBackend` round-trip against embedded NATS (`testutil/natstest`), `ErrNotFound` mapping via `errors.Is`, Delete-absent-not-error; plus a reusable failing-`Backend` stub for error-path tests in later phases

**Checkpoint**: the boundary compiles; every story builds on it.

## Phase 3: User Story 1 — State survives a restart (Priority: P1) 🎯 MVP

**Goal**: write-through durability with lazy, codec-equal restore across a real imp restart.

**Independent Test**: mutate an entity through a running imp, stop it, start a fresh instance, and read the entity back equal under the codec — no replay step, no flush step.

- [x] T005 [US1] Implement `persist/store.go`: `Store[T]`, `NewStore(name, backend, opts...)` with `WithBound` (default 256, panic on n ≤ 0), `WithWake`, `WithCodec` (default JSON); the envelope (`state` bytes + `last_active`, one JSON value, key `<name>.<entity>`); `Get` (resident hit / rehydrate-with-wake / zero-value never-seen / error surfacing), `Update` (rehydrating Get → fn → write-through → residency refresh), `Delete` (residency + backend), `Resident`; LRU residency with pure-drop eviction; one mutex held across IO; unexported injectable `now`, per `contracts/go-api.md` and research.md D2–D6
- [x] T006 [P] [US1] Unit tests in `persist/store_test.go` (part 1): Update-then-raw-backend-read shows the envelope durable before return (write-through); never-seen entity yields zero `T`, no error; failing backend surfaces errors from both `Get` and `Update` (never a silent zero); `Delete` clears residency and backend while eviction alone never deletes; codec override applied (spec US1 scenarios 2–4; SC-006)
- [x] T007 [US1] Integration test in `persist/restart_test.go`: the research spike productized — a real imp (`SubjectSource` channel, awareness doing `store.Update` per message) against embedded NATS + KV; stop the imp, start a fresh instance with a fresh store, first access returns state equal under the codec to the pre-stop value with zero startup work (spec US1 scenario 1; SC-001)

**Checkpoint**: US1 is the shippable MVP — durable state across restarts.

## Phase 4: User Story 2 — Time-dependent state wakes correctly (Priority: P2)

**Goal**: exactly-once per-entity wake with true elapsed, pre-visibility; the imp-level Beacon gate.

**Independent Test**: with an injected clock, prove exactly-once/elapsed/pre-visibility deterministically; across a real stop, prove elapsed ≥ the stop; prove Beacon first-start absence and measured slept-for.

- [x] T008 [US2] Unit tests in `persist/store_test.go` (part 2), using the injected clock: wake fires exactly once per rehydration with elapsed = now − last_active; the state `fn` sees in `Update` is already woken (pre-visibility); resident hits never fire; evict-then-access fires again with the genuine interval since last activity (no write-back semantics); concurrent `Get`s for the same entity under `-race` fire the wake exactly once (spec US2 scenarios 1–3; SC-002; FR-011)
- [x] T009 [US2] Implement `persist/beacon.go`: `Beacon`, `NewBeacon(name, b)`, `Stamp` (now under key `<name>`), `SleptFor` → `(elapsed, ok, err)` with never-stamped → `ok=false` (absence, not zero), backend failures → error, per `contracts/go-api.md`
- [x] T010 [P] [US2] Unit tests in `persist/beacon_test.go`: first-ever start reports `ok=false`; stamp → slept-for ≥ the (injected-clock) interval; failing backend surfaces an error (spec US2 scenario 4; SC-004)
- [x] T011 [US2] Extend `persist/restart_test.go`: across the real stop, the wake hook's elapsed is ≥ the measured stop duration and wall-clock-bounded, and a `Beacon` stamped at shutdown reports a matching slept-for on the fresh instance (SC-002, SC-004)

**Checkpoint**: time-dependent state is safe across sleep at both levels.

## Phase 5: User Story 3 — Memory stays bounded without losing state (Priority: P3)

**Goal**: the bound holds, nothing is rejected, nothing is lost.

**Independent Test**: touch N > bound entities; residency ≤ bound throughout; all N read back correct.

- [x] T012 [US3] Unit tests in `persist/store_test.go` (part 3): with bound 4 and 10 entities, residency never exceeds 4 during writes or readback and every entity reads back correct after eviction (rehydrated); eviction performs no backend writes or deletes (assert via a counting-`Backend` wrapper); new entities are never rejected at the bound (spec US3 scenarios 1–3; SC-003; FR-006)

**Checkpoint**: full M2 behavior complete.

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T013 [P] Keep `specs/005-sleep-wake-persistence/quickstart.md` in sync with the implemented API (compile-check its example shape; the contract file governs — drift beyond the contract must be surfaced, not silently adopted)
- [x] T014 Byte-identity verification per `contracts/repo-gate.md`: `git diff main -- go.mod go.sum` empty; no root-package `*.go` modified; no `Makefile` or CI diff; the branch's changed-file list confined to `persist/`, `specs/005-sleep-wake-persistence/`, `hq/`, `CLAUDE.md` (SC-005)
- [x] T015 Full gate: `make fmt && make test && make lint` plus `make compile-deny` — green across both modules, zero skipped tests (SC-006)
- [x] T016 Landing duties in the same change (hq/00-GENESIS/how-we-work.md): move M2 to the roadmap ledger with the outcome, write the journey episode via `/journey-log`, propagate any behavioral drift back into `hq/02-DESIGN/0004-sleep-wake-persistence.md` and flip the anatomy's Memory/Persistence/wake-hook tags to `[V]`, refresh `hq/04-JOURNEY/README.md` "Where things stand", set the spec status to Shipped

## Dependencies & Execution Order

- **Phase 1–2 → stories**: T001 any time; T002 blocks T004/T005; T003 blocks T005.
- **US1 (T005–T007)**: T005 → T006 ∥ T007. Delivers the MVP alone.
- **US2 (T008–T011)**: T008 needs T005; T009 independent of the store (needs T002); T010 after T009; T011 after T007+T009.
- **US3 (T012)**: needs T005 only.
- **Polish (T013–T016)**: T013 after APIs settle; T014/T015 last before T016.

**Parallel opportunities**: T003 ∥ T004 after T002; T006 ∥ T007 after T005; T008 ∥ T009 ∥ T012 after T005; T010 ∥ T011 after T009.

## Implementation Strategy

MVP first: T001–T007 ships durable restart survival (US1). US2 adds the wake
contract at both levels; US3 closes boundedness. The integration file grows
around one scaffold (embedded NATS + KV + a real imp) mirroring the research
spike; unit determinism comes from the injected clock, not sleeps. Land only
at T016 with the gate green and the hq duties in the same change.
