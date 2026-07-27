# Phase 0 Research: Sleep, Wake, and Per-Entity Persistence

All material unknowns were resolved by the `sleep-wake-persistence` research
topic (pre-registered bars, seam inventory, spike, adversarial pass —
[episode 0005](../../hq/04-JOURNEY/0005-sleep-wake-persistence.md)) and its
graduated design doc
([`hq/02-DESIGN/0004-sleep-wake-persistence.md`](../../hq/02-DESIGN/0004-sleep-wake-persistence.md)).
This file consolidates them plus the plan-time build decisions. No NEEDS
CLARIFICATION markers remain.

## D1. Placement: beside the registry, as a core-module package

- **Decision**: `imps/persist`, a plain package in the core module. The
  registry (`ImpSpec.States`) stays the ephemeral hot tier, untouched.
- **Rationale**: riding the registry is blocked four ways by documented
  guarantees (cap-rejection contract, error-less `Get`, entity-less
  `Factory`, no enumeration — Bar 1 `[measured]`); a nested module would
  fence dependencies that don't exist (`jetstream` is already required).
- **Alternatives considered**: harness-native registry extension (refuted in
  the adversarial pass — breaks documented guarantees, moves backend IO into
  the dispatch path); nested module (refuted at design time — no deps to
  fence).

## D2. Durability model: write-through; the snapshot is continuous

- **Decision**: `Update` returns only after the backend accepted the
  envelope. No flush-at-shutdown, no snapshot schedule.
- **Rationale**: measured in the spike — stopping the imp *is* sleeping;
  eviction becomes a lossless drop by construction.
- **Alternatives considered**: scheduled snapshots (a window of loss plus a
  schedule to tune — refuted); write-back batching (named upgrade path if
  update-rate evidence appears; not built speculatively).

## D3. Restore model: lazy, on access; wake before visibility

- **Decision**: nothing loads at startup. First access rehydrates; the wake
  hook (if configured) runs with elapsed = now − last-active before the
  state is returned. Never-seen entities yield the zero state, no wake, no
  error. Wake does not write back; re-fire after eviction recomputes from
  the same stamp, so hooks are documented as "advance to now" pure
  functions.
- **Rationale**: cold starts stay fast and small; the spike measured
  exactly-once per rehydration and deterministic advancement.
- **Alternatives considered**: startup replay (slow cold starts, needs
  enumeration the boundary forbids); wake write-back (turns every read into
  a write; rejected — the no-write-back semantics are harmless under the
  documented hook contract).

## D4. Envelope encoding: `state` as raw bytes, not embedded JSON

- **Decision**: the envelope is JSON `{state: <bytes>, last_active:
  <RFC3339Nano>}` where `state` is the codec's output carried as a byte
  field (base64 in the JSON encoding). Key: `<store-name>.<entity>`.
- **Rationale**: the codec is replaceable (FR-010); embedding codec output
  as a JSON raw message would silently require every codec to emit valid
  JSON. Bytes-as-bytes keeps the codec contract honest. (A design-time
  refinement of the spike, which used a raw-message field with the JSON
  codec only — recorded here so the drift is explicit.)
- **Alternatives considered**: `json.RawMessage` state (couples all codecs
  to JSON); two backend keys per entity (state + stamp — two round-trips
  and a torn-write window; refuted).

## D5. Concurrency: one store mutex, held across backend IO

- **Decision**: store operations serialize under a single mutex, including
  the backend round-trip.
- **Rationale**: the wake-exactly-once guarantee requires per-entity
  serialization anyway; a single mutex is the simplest shape that makes the
  guarantee hold under `-race` concurrency tests. Dispatch is
  single-goroutine per imp, so the practical contention is thinking-vs-
  awareness only.
- **Alternatives considered**: per-entity locks (the named upgrade if
  contention evidence appears — more machinery, same contract); lock-free
  resident reads (unsound with LRU mutation on every hit).

## D6. Clock: injectable, unexported

- **Decision**: the store and beacon read time through an internal `now
  func() time.Time`, settable from same-package tests only.
- **Rationale**: Bar-3-grade elapsed assertions become deterministic in unit
  tests; the restart integration test uses the real clock with wall-clock
  bounds, as the spike did.
- **Alternatives considered**: exported `WithClock` option (public API
  surface for a test seam — refuted; the design doc's option set is the
  contract).

## D7. Beacon: explicit stamp, explicit read; absence is not zero

- **Decision**: `Beacon.Stamp(ctx)` writes an imp-scoped stamp under the
  beacon's own key; `SleptFor(ctx)` returns `(elapsed, ok, err)` where
  `ok=false` means never stamped. The pre-`Run` gate is a documented usage
  pattern in `main()`, not a harness hook.
- **Rationale**: the harness cannot know it slept; the honest elapsed source
  is a stamped clock, and the anatomy's "single call before dispatch
  resumes" is satisfied by sequencing in `main()` (episode 0005
  `[mechanism-argument]`).
- **Alternatives considered**: automatic stamping from inside the store
  (couples the imp-level clock to entity traffic — an idle imp would look
  asleep); a harness `OnWake` field (a harness change this feature is
  defined by not needing).

## D8. Test strategy: the spike productized, plus determinism

- **Decision**: `restart_test.go` reproduces the research spike as a real-imp
  integration test (stop → fresh instance → codec-equal + wake elapsed);
  `store_test.go` covers bound/no-loss (Bar 4 shape), exactly-once wake with
  an injected clock, zero-value and error paths, and `-race` concurrency;
  `beacon_test.go` covers first-start absence and measured slept-for;
  `backend_test.go` covers `ErrNotFound` mapping and a failing-backend stub
  (SC-006's error-never-silent-zero).
- **Rationale**: the spike is measured ground truth; unit determinism comes
  from D6 rather than sleeps wherever possible.
- **Alternatives considered**: mocking the backend everywhere (the KV
  adapter would go untested; the embedded server is already a core test
  dependency).
