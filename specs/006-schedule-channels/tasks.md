# Tasks: Schedule Channels

**Input**: Design documents from `/specs/006-schedule-channels/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/go-api.md, contracts/repo-gate.md, quickstart.md

**Tests**: included — the success criteria are measured readings and the spike becomes the permanent suite (research.md D6).

**Organization**: tasks grouped by user story. All code lives in the NEW `schedule/` package of the core module; the root harness package, `go.mod`, `Makefile`, and CI are untouched by every task (FR-001, contracts/repo-gate.md).

## Phase 1: Setup

- [x] T001 Write `schedule/doc.go` per the `contracts/go-api.md` documentation contract: the server owns the clock (no timers, no tick production, no registry in the framework); registration is thinking/operator-tier — never hand awareness a jetstream handle; TTL absence means full accumulation, stated plainly; stream provisioning is the operator's act

## Phase 2: Foundational

- [x] T002 Implement `schedule/tick.go`: the `Tick{Subject, Scheduler, Next}` type, the header-only default decoder (message subject + `Nats-Scheduler` + `Nats-Schedule-Next`, verbatim), the default entity extractor (target subject), the default channel name (`"schedule:"+target`), and the package's single definition site for the substrate header names, per `contracts/go-api.md` and research.md D3/D5

**Checkpoint**: every story builds on `Tick` and the header constants.

## Phase 3: User Story 1 — Periodic work arrives as an ordinary channel (Priority: P1) 🎯 MVP

**Goal**: ticks reach awareness through the unmodified dispatch path, live and as durable catch-up.

**Independent Test**: registered schedule + one-channel imp: live ticks with provenance; durable catch-up after a cold gap.

- [x] T003 [US1] Implement `schedule/channel.go`: `Channel(stream, target, ...ChannelOption) imps.ChannelSpec` on the existing `imps.StreamSource` (target as filter subject verbatim, deliver-all default) with `WithDurable`, `WithStartSeq`, `WithStartTime`, `WithDecoder`, `WithEntityExtractor`, `WithName`, per `contracts/go-api.md`
- [x] T004 [P] [US1] Unit tests in `schedule/channel_test.go`: spec construction defaults (stream/filter/deliver-all/name), each option's effect, header-only decode incl. missing headers → zero-valued fields with no error (spec US1 scenario 3)
- [x] T005 [US1] Integration scaffolding + warm path in `schedule/schedule_test.go`: embedded NATS, stream with `AllowMsgSchedules`+`AllowMsgTTL` (operator stand-in), a registered `@every 1s` schedule, an imp on `schedule.Channel` with a durable: live ticks reach awareness with `Tick.Scheduler` naming the producer; metrics show zero decode/extraction failures (spec US1 scenario 1; SC-001)

**Checkpoint**: US1 is the MVP — an imp on the clock.

## Phase 4: User Story 2 — Registering and removing schedules, typed (Priority: P2)

**Goal**: every option→header mapping exact; replacement and removal are the substrate's semantics surfaced.

**Independent Test**: register with all options, read the stored schedule back header-by-header; replace; deregister; fail fast on invalid input.

- [x] T006 [US2] Implement `schedule/register.go`: `Register(ctx, js, scheduleSubject, pattern, target, ...RegisterOption)` publishing exactly one message with exactly the headers the options imply (`WithTickTTL` — Go-duration formatted, panic-free validation `>0` → error; `WithTimeZone`, `WithBody`, `WithSource`, `WithRollup`); fail-fast errors on empty pattern/target before substrate contact; `Deregister(ctx, js, stream, scheduleSubject)` as subject purge, per `contracts/go-api.md` and research.md D4
- [x] T007 [US2] Integration tests in `schedule/register_test.go`: all-options register → `GetLastMsgForSubject` read-back asserts every implied header present and correctly formatted and no other scheduling header; minimal register → no TTL/zone/source/rollup headers; re-register with a new pattern → stored schedule replaced (same subject, new header); deregister → schedule subject gone, previously emitted ticks untouched; empty pattern/target and non-positive TTL → errors with zero substrate contact (spec US2 scenarios 1, 3, 4; SC-003)
- [x] T008 [US2] Integration test in `schedule/register_test.go`: replacement takes effect on firing — register `@every 1h` (no tick within a short observation window), re-register the same subject as `@every 1s`, a tick arrives within seconds (fast-direction switch only, per research.md D6; spec US2 scenario 2; SC-003)

**Checkpoint**: full typed registration lifecycle proven.

## Phase 5: User Story 3 — Stale ticks governed by an explicit TTL (Priority: P3)

**Goal**: the roadmap's exit criterion — accumulation governed server-side, both directions, exact counts.

**Independent Test**: TTL and no-TTL schedules through the same cold gap; counts compared.

- [x] T009 [US3] Integration test in `schedule/schedule_test.go`: the research spike's cold phase productized — two `@every 1s` schedules (one `WithTickTTL(2s)`, one without), imp stopped ~5 s while both fire, restarted: the no-TTL channel delivers the full backlog (≥3), the TTL channel strictly fewer (the unexpired tail, 1..4), all through durable catch-up on the same seam with zero imp-side filtering (spec US3; SC-001, SC-002)

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T010 [P] Keep `specs/006-schedule-channels/quickstart.md` in sync with the implemented API (contract file governs; drift surfaced, never silently adopted)
- [x] T011 Byte-identity per `contracts/repo-gate.md`: `git diff main -- go.mod go.sum` empty; no root `*.go`, `Makefile`, or CI diff; changed files confined to `schedule/`, `specs/006-schedule-channels/`, `hq/`, `CLAUDE.md` (SC-004)
- [x] T012 Full gate: `make fmt && make test && make lint` plus `make compile-deny` — green across both modules, zero skipped tests (SC-005)
- [x] T013 Landing duties in the same change (hq/00-GENESIS/how-we-work.md): roadmap M3 → ledger (front → M4), journey episode via `/journey-log`, design doc 0005 → `[V]` with any drift propagated, anatomy schedule-channels `[D]` → `[V]`, journey README index + "Where things stand" refreshed, spec status Shipped

## Dependencies & Execution Order

- T001 any time; T002 blocks everything after it.
- **US1**: T003 → T004 ∥ T005 (T005 needs T006's `Register` for scaffolding — implement T006 before or inline a raw-header publish in the scaffold and swap after; preferred order: T003 → T006 → T005).
- **US2**: T006 → T007 ∥ T008.
- **US3**: T009 needs T003+T006 and reuses T005's scaffold.
- **Polish**: T010 after APIs settle; T011/T012 last before T013.

**Parallel opportunities**: T004 ∥ (T006, T007) after T003/T002; T008 ∥ T009 after T006; T010 ∥ T011 at the end.

## Implementation Strategy

MVP first (T001–T005 with T006 pulled forward as the scaffold's registrar),
then the typed-registration lifecycle, then the TTL criterion. One
integration scaffold (embedded server + flagged stream) mirrors the research
spike; timing assertions use generous bounds and fast-direction switches
only. Land only at T013 with the gate green and the hq duties in the same
change.
