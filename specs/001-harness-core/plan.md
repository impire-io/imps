# Implementation Plan: Harness Core

**Branch**: `001-harness-core` | **Date**: 2026-05-10 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-harness-core/spec.md`

## Summary

The harness core delivers the minimal in-process Go substrate that holds an imp together: declarative `ImpSpec` construction, channel subscription (subject and stream), structurally-separated `AwarenessContext` / `ReasoningContext` interfaces, per-entity local memory with bounded capacity, and action publishing constrained by a declared whitelist. The harness is one Go module (`github.com/impire-io/imps`) with a single public `harness` package and module-private `internal/` packages for dispatch, state, stream binding, subject resolution, and observability. Capabilities, the soulstream, schedule channels, KV channels, persistence, and audit are explicitly out of scope and ship as separate features (FR-NS-1, FR-NS-2).

The energy gradient is enforced structurally: `AwarenessContext` has no `Publish` method, so calling `awareness.Publish(...)` does not compile (FR-014, SC-006); the contract is asserted at build time by an `integration/compiletest` file. The reasoning context's `Publish` rejects off-whitelist subjects with a typed `ErrWhitelistViolation` *before* any substrate call (FR-027). The `Verdict` sum type (`Ignore()`, `Note(payload)`, `Wake(reason, entity)`) drives the dispatch state machine; reasoning runs in its own goroutine per `Wake` so awareness never blocks (FR-016, FR-020). Stream channels ack after awareness completes regardless of reasoning lifetime (FR-008a, clarification Q2). All tests run against an embedded `nats-server/v2` (JetStream-enabled per test) — no external infrastructure is required.

The full set of design decisions is in [`research.md`](./research.md); the runtime types are in [`data-model.md`](./data-model.md); the binding contracts are under [`contracts/`](./contracts/) (public API, observability, stream-channel, subject resolution); and the imp-author walkthrough is in [`quickstart.md`](./quickstart.md).

## Technical Context

**Language/Version**: Go 1.25 (research R-1).
**Primary Dependencies**:
- `github.com/nats-io/nats.go` — core NATS subscriptions, JetStream consumers, publish (R-2).
- `github.com/nats-io/nats-server/v2` — embedded server for tests (in-process, JetStream-enabled per test, R-4). Helper at `testutil/natstest`.
- `github.com/synadia-io/orbit.go/natscontext` — context-based connection loading for the example/CLI and any test fixture that connects to a developer-local NATS (R-3, repository CLAUDE.md mandate). The harness library itself accepts a `*nats.Conn` from the caller and does not import `natscontext`.
- `log/slog` (Go standard library) — structured logging (R-5).
- Go standard library only for sync, atomic, context, time.

**Storage**: In-process only for this feature. Per-entity state is held in a typed registry — one `sync.Map` of `Entity → *stateBox` per state shape, with an atomic per-shape entity counter to enforce caps (R-8). No disk, no external store. Persistence/snapshotting and rehydration are deferred to the sleep/wake feature (FR-NS-2).

**Testing**: Go standard `testing` package with table-driven subtests. End-to-end tests boot an embedded `nats-server/v2` (JetStream enabled when stream channels are exercised) via `testutil/natstest` (R-4, R-12). No mocking of NATS — every test exercises the real subscription, dispatch, and publish path. The compile-time invariant from FR-014 / SC-006 is asserted by a build-tagged file under `integration/compiletest/` whose presence-of-build-failure is the assertion (`contracts/public-api.md` §"Compile-time guarantees"). Run discipline: `go test -race -count=1 ./...` (R-17).

**Target Platform**: Linux and macOS, single-process Go runtime. The harness is in-process with the imp's awareness and reasoning code (spec Assumption: "The harness runs in-process with the imp's awareness and reasoning code").

**Project Type**: Go library (single module, single public package + internal sub-packages). The developer-facing import is one package — `github.com/impire-io/imps/harness` — by deliberate design (R-16).

**Performance Goals**:
- End-to-end channel→awareness→reasoning→action round-trip under 1 s on a local embedded NATS server (SC-002).
- Awareness latency under reasoning load: ≤ 50 ms additional vs. unblocked baseline (SC-003).
- Concurrent reasoning for K ≥ 100 distinct entities without harness-attributable serialization (SC-004).
- Per-message dispatch overhead bounded and sub-linear in (in-flight reasoning count, tracked entity count) (SC-010).
- Sustained awareness dispatch with thousands of entities tracked and reasoning in flight (SC-011).

**Constraints**:
- No bound on in-flight reasoning concurrency (FR-021a). The harness exposes the count via `ReasoningContext.InFlight()` and `Imp.Metrics().InflightReasoning` (FR-021b) but does not throttle.
- Stream-channel ack happens at awareness completion regardless of reasoning lifetime (FR-008a, clarification Q2).
- `AwarenessContext` MUST NOT expose `Publish` — structural absence, not a runtime check (FR-014).
- No retry / circuit-breaker / dead-letter logic for any failure path (FR-NS-3).
- No cross-imp shared memory; every `(name, entity)` slot lives inside one imp instance (FR-NS-4).

**Scale/Scope**: A single imp instance must sustain continuous awareness dispatch on a channel while reasoning is in flight for thousands of distinct entities (SC-011). The harness module is intentionally small — single public package, ~five internal sub-packages keyed to dispatch / state / stream / subject / observability — and the contract surface fits on one screen (`contracts/public-api.md`).

## Constitution Check

*Gate evaluated against the Load-Bearing Commitments and Non-Negotiables in `.specify/memory/constitution.md` v2.0.0.*

### Load-Bearing Commitments

| Commitment | Pass? | Evidence |
|---|---|---|
| Imps stay small and agile | ✅ | This feature is *only* the substrate. Capabilities, soulstream, schedules, KV, persistence, audit are explicitly deferred (FR-NS-1, FR-NS-2). The harness module ships one public package; the surface is the FRs and nothing else. |
| The energy gradient is structural | ✅ | `AwarenessContext` and `ReasoningContext` are distinct interfaces (R-7); `Publish` is defined only on `ReasoningContext` (FR-014). The compile-time invariant is asserted by a build-tagged file under `integration/compiletest/` (`contracts/public-api.md` §"Compile-time guarantees"). Whitelist enforcement runs at the publish call site before the substrate (FR-027). |
| Capabilities are external; the harness is small | ✅ | This feature provides no capability implementations and no capability client. The reasoning context exposes only `State`, `Publish`, and `InFlight` (FR-017, observability contract). FR-NS-1 makes "no bounded-capability surface in awareness" explicit for v1. |
| Coordination happens through the soulstream | N/A here | This feature does not introduce inter-imp coordination. Soulstream channels are explicitly deferred (FR-NS-2). The action-publish surface is generic NATS publish constrained by whitelist; soulstream conventions layer on later. |
| Wire protocols are per-capability; deployment shape is uniform | N/A here | No capability is being designed in this feature. The platform-mode subject convention (FR-030/FR-031, `contracts/subject-resolution.md`) is the deployment-shape contract this feature commits to and will be reused by every capability service that follows. |

### Non-Negotiables

| Rule | Pass? | Evidence |
|---|---|---|
| Awareness does not call unbounded capabilities | ✅ | `AwarenessContext` exposes only `State(name, entity)` and the verdict return path (FR-014, `contracts/public-api.md` §"Awareness"). No capability surface exists in this feature, bounded or unbounded (FR-NS-1). |
| Imps do not share local memory | ✅ | State is keyed by `(state name, entity)` within a single imp instance (FR-022/023); FR-NS-4 makes the prohibition on cross-imp shared memory explicit. |
| Capability services do not persist per-request data | N/A here | No capability service is built in this feature. |
| Direct provider/SDK calls in imp code are forbidden | ✅ | The harness only depends on the NATS client; no LLM SDK, no DB driver. Imp authors using this harness publish to NATS subjects only. |
| No central registry beyond NATS micro | ✅ | `Imp.Identity()` is in-process only — not registered with NATS in v1 (`contracts/observability.md` §"Probing identity"). Discovery via `$SRV.INFO` belongs to a future capability/discovery feature. |
| No generic capability protocol | ✅ | No capability protocol exists in this feature. |
| Stubs and partial implementations are never reported as complete | ✅ commitment | Implementation discipline. The plan does not include stubs as deliverables. Tests assert behavior; any unfinished branch will be reported as partial per the constitution's "Done means done" rule. |

**Gate result**: **PASS** — both pre-research and post-design (re-checked after producing `data-model.md` and `contracts/`). No justified violations; the Complexity Tracking table below is empty.

## Project Structure

### Documentation (this feature)

```text
specs/001-harness-core/
├── plan.md                          # This file (/speckit-plan output)
├── research.md                      # Phase 0 — 17 design decisions with rationale
├── data-model.md                    # Phase 1 — runtime types, validation rules, lifecycle
├── quickstart.md                    # Phase 1 — imp-author walkthrough (echo imp + variants)
├── contracts/
│   ├── public-api.md                # Go-level developer-facing surface
│   ├── observability.md             # Metrics, OnNote hook, slog events, what is NOT exposed
│   ├── stream-channel.md            # JetStream startup, ack/NAK, durable compatibility
│   └── subject-resolution.md        # Both deployment modes; symmetry guarantees
├── checklists/
│   └── requirements.md              # Spec-quality checklist (already passed)
└── tasks.md                         # /speckit-tasks output — present from a prior run; this command does NOT modify it
```

### Source Code (repository root)

```text
go.mod                               # module github.com/impire-io/imps, go 1.25
go.sum
Makefile                             # fmt, tidy, test, lint, build, check (R-17)
.golangci.yml                        # errcheck, govet, staticcheck, unused, gofmt, goimports, revive

harness/                             # public package — the developer-facing API
├── spec.go                          # ImpSpec, validation at NewImp (FR-001, FR-002, FR-003)
├── channel.go                       # ChannelSpec, Source/SubjectSource/StreamSource (FR-004*)
├── verdict.go                       # Verdict sum: Ignore() / Note(p) / Wake(r,e) (FR-010..013)
├── context.go                       # AwarenessContext + ReasoningContext interfaces (FR-014, FR-017, FR-029)
├── state.go                         # StateShape, StateRef interface, typed errors (FR-022..026)
├── imp.go                           # Imp handle, NewImp, Run, Shutdown, Identity, Metrics
├── options.go                       # Option funcs: WithDrainWindow, WithLogger, WithSubjectPrefix, WithPlatformMode
├── errors.go                        # Typed errors (ErrSpecInvalid, ErrCapExceeded, ErrWhitelistViolation, ...)
├── message.go                       # Message struct (Subject, Reply, Headers, Data) — substrate-agnostic view
└── doc.go                           # Package-level godoc

internal/
├── dispatch/                        # channel→decode→extract→awareness→reasoning pipeline
│   ├── dispatcher.go                # core dispatch loop, panic recovery (FR-006..008, FR-015, FR-021)
│   ├── reasoning.go                 # goroutine-per-Wake launcher; atomic in-flight counter (FR-016, FR-018..021b)
│   └── notes.go                     # Note-record fan-out to OnNote hook
├── state/
│   └── store.go                     # per-(shape,entity) instances, per-shape cap (FR-022..026, R-8)
├── stream/
│   └── consumer.go                  # JetStream bind/create + compatibility check (FR-005a..c, FR-008a, R-9, R-10)
├── subject/
│   └── resolve.go                   # platform-mode and prefix subject resolution (FR-030..033, R-11)
├── obs/
│   └── metrics.go                   # atomic counters; Metrics() snapshot (FR-021b, observability contract)
├── ack/
│   └── ack.go                       # stream-channel ack/NAK helpers tied to awareness verdict completion
└── lifecycle/
    └── lifecycle.go                 # Created/Starting/Running/Draining/Stopped/Failed state machine (FR-034..036, R-14)

integration/                         # black-box tests of the public harness package against embedded NATS
├── dispatch_test.go                 # US-1, US-2, US-6
├── stream_test.go                   # US-4
├── boundary_test.go                 # US-3 runtime checks (whitelist violation, etc.)
├── state_test.go                    # US-5 (cap, unknown shape, concurrency)
├── modes_test.go                    # US-7 (non-platform / platform symmetry)
├── lifecycle_test.go                # US-8 (clean start/shutdown, drain window)
└── compiletest/
    └── awareness_publish_absence_test.go    # build-tagged; build failure = passing assertion of FR-014 / SC-006

testutil/
└── natstest/
    └── server.go                    # embedded nats-server bootstrap with JetStream toggle, t.Cleanup-driven

examples/
└── echo/                            # the quickstart imp, runnable against `nats-server`
    └── main.go

docs/                                # already present (vision, anatomy, capability service pattern)
specs/                               # already present (this feature)
```

**Structure Decision**: Single Go module (`github.com/impire-io/imps`) with one public package (`harness/`) and a small set of `internal/` sub-packages keyed to the responsibilities the spec calls out: dispatch, state, stream, subject resolution, observability, ack, and lifecycle. Black-box tests live under `integration/`; the compile-time FR-014 invariant lives in a build-tagged file under `integration/compiletest/`; the embedded NATS helper is in `testutil/natstest/`; the runnable quickstart imp is in `examples/echo/`. This layout honors the constitution's "imps stay small and agile" — the developer-facing import is *one* package — and its "operational shape over architectural elegance" — internal packages are split by *what could fail or change* (a JetStream-API bump touches `internal/stream/` only; a subject-convention change touches `internal/subject/` only) rather than by abstract layers. It also avoids the legacy projection/derivation/reactor decomposition that the spec explicitly rejects (`docs/01-harness-anatomy.md`, R-16).

## Phase 2: task strategy (preview only — `/speckit-tasks` is a separate command)

`/speckit-plan` stops at Phase 1. The task plan below is descriptive — to be expanded by `/speckit-tasks` into the per-task `tasks.md`. (A `tasks.md` from a prior run already exists in this directory; it is left untouched by this command.) The expected task strategy follows the user-story dependency order:

1. **US-1 (P1) — End-to-end imp dispatch.** Public types (`ImpSpec`, `Verdict`, `AwarenessContext`, `ReasoningContext`, `Entity`, `StateRef`, `Source`/`SubjectSource`); `NewImp` validation; subject resolver (non-platform mode only); `internal/dispatch` core loop; `Imp.Run` / `Imp.Shutdown`; `integration/dispatch_test.go` golden path.
2. **US-2 (P1) — Verdict semantics.** Fully implement `Ignore`/`Note`/`Wake` paths in dispatch; OnNote hook; reasoning goroutine launcher with WaitGroup + atomic in-flight counter.
3. **US-3 (P1) — Structural boundary.** `integration/compiletest/awareness_publish_absence_test.go` (must fail to build if `Publish` ever appears on `AwarenessContext`); `internal/dispatch` whitelist check + `ErrWhitelistViolation`; `boundary_test.go` runtime assertions.
4. **US-4 (P1) — Stream channels.** `StreamSource`, `ConsumerConfig`, `internal/stream/consumer.go` (bind/create/compat-check), JetStream startup error mapping, `internal/ack/ack.go`, `integration/stream_test.go`. Embedded server in `testutil/natstest` gains a JetStream toggle.
5. **US-5 (P2) — Local memory.** `internal/state/store.go` per-shape `sync.Map` + atomic counter; `StateRef` interface (`Get`/`Set`/`Update`); cap-exceeded and unknown-shape errors; `state_test.go` including concurrent same-entity write.
6. **US-6 (P2) — Reasoning concurrency.** Already enabled by US-2's launcher; this story is a test-only story plus the `InFlight()` accessor on `ReasoningContext` and the matching counter exposure (FR-021b).
7. **US-7 (P2) — Both deployment modes.** `internal/subject/resolve.go` platform-mode branch and config validation (`ErrConfigInvalid` for missing `importer_account_pk`); `modes_test.go` driving the same imp through both modes.
8. **US-8 (P3) — Clean lifecycle.** `internal/lifecycle/lifecycle.go` state machine (Created → Starting → Running → Draining → Stopped, plus terminal Failed); two-phase shutdown (R-14); ephemeral-consumer teardown; `lifecycle_test.go`.

Cross-cutting tasks (run alongside the user stories, not blocking them):
- `Makefile`, `.golangci.yml`, `go.mod` initialization (R-17).
- `internal/obs/metrics.go` atomic counters; `Imp.Metrics()` snapshot (FR-021b, observability contract).
- `examples/echo/main.go` matching `quickstart.md` exactly.
- Logger plumbing via `slog.Handler` option (R-5).

The `/speckit-tasks` command will derive the actual `tasks.md` from this strategy plus `data-model.md` and the four contract files; the present plan only constrains the high-level shape.

## Complexity Tracking

> No constitutional violations require justification. Table intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
