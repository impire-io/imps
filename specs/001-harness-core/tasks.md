---

description: "Task list for implementing Harness Core"
---

# Tasks: Harness Core

> **Note (2026-05-11)**: After this feature shipped, the constitution was
> amended in two passes (v2.1.0 then v2.2.0) refining the "Imps see one
> subject path" principle. In the same coordinated cleanup:
>   - `WithPlatformMode(importerAccountPK)` (T015) and the platform-mode
>     resolver branch (T018 / T070–T075) were removed.
>   - The remaining `WithSubjectPrefix(prefix)` option and the
>     single-form resolver were also removed; the framework now performs
>     no subject transformation at all. A declared subject is the
>     substrate subject verbatim.
>   - `ImpIdentity.SubjectPrefix` was dropped; `Imp.Ready() bool` was
>     added as the post-startup readiness signal that tests previously
>     polled via the prefix.
>   - The `ImpSpec.Actions` whitelist, `*ErrWhitelistViolation`, the
>     `WhitelistViolations` metric, and the related runtime check were
>     removed. Subject permissioning is the substrate's concern
>     (NATS account ACLs).
>   - `ReasoningContext.Conn() *nats.Conn` was added as the escape
>     hatch for generic NATS-based clients used from reasoning.
>     Awareness has no equivalent method — the absence is the
>     structural enforcement of the energy gradient.
> Cross-account access is the substrate's concern (NATS account
> imports). The task entries below remain as the historical
> implementation record.

**Input**: Design documents from `/specs/001-harness-core/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks ARE included. Each User Story in spec.md ships with an Independent Test, and plan.md maps each story to a dedicated `integration/*_test.go` file. Treat integration tests as the spec's acceptance harness — write them first, watch them fail, then implement.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing. Each user story phase ends in a checkpoint where that story can be demonstrated end-to-end against an embedded NATS server.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, …, US8)
- File paths are exact; the task is "create or extend that file"

## Path Conventions

Single Go module rooted at `github.com/impire-io/imps` (per `plan.md` Structure Decision):

- Public surface: `harness/`
- Implementation: `internal/`
- Integration tests: `integration/`
- Test helpers: `testutil/natstest/`
- Worked example: `examples/echo/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Bring the Go module, build tooling, and dependency surface online.

- [X] T001 Initialize Go module at repo root with `go mod init github.com/impire-io/imps` and Go 1.25 toolchain directive in `go.mod`
- [X] T002 [P] Create top-level package directories `harness/`, `internal/dispatch/`, `internal/state/`, `internal/subjects/`, `internal/stream/`, `internal/lifecycle/`, `internal/observability/`, `integration/`, `testutil/natstest/`, `examples/echo/`
- [X] T003 [P] Add `harness/doc.go` package documentation describing the public surface (one paragraph + reference to `specs/001-harness-core/contracts/public-api.md`)
- [X] T004 [P] Add dependencies to `go.mod` and run `go mod tidy`: `github.com/nats-io/nats.go`, `github.com/nats-io/nats-server/v2` (test-only), `github.com/synadia-io/orbit.go/natscontext` (example/test only)
- [X] T005 [P] Create `Makefile` at repo root with targets `fmt`, `tidy`, `test` (runs `go test -race -count=1 ./...`), `lint` (runs `golangci-lint run ./...`), `build`, and `check` (composes fmt+tidy+test+lint)
- [X] T006 [P] Create `.golangci.yml` at repo root enabling `errcheck`, `govet`, `staticcheck`, `unused`, `gofmt`, `goimports`, `revive` (per research.md R-17)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Public types, errors, and core internal infrastructure that every user story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T007 [P] Define `Entity` typed string in `harness/spec.go` with empty-string validation contract (per public-api.md and FR-007)
- [X] T008 [P] Define `ImpSpec`, `ImpIdentity`, `Metrics`, `StateShape` structs and `AwarenessFn`/`ReasoningFn` function types in `harness/spec.go` (per public-api.md "ImpSpec", "Lifecycle", and data-model.md "ImpSpec"/"StateShape"/"ImpIdentity"/"Metrics")
- [X] T009 [P] Define `Verdict` closed-sum type (unexported discriminator) plus exported constructors `Ignore()`, `Note(payload any)`, `Wake(reason any, entity Entity)` in `harness/verdict.go` (per public-api.md "Verdict" and research.md R-6)
- [X] T010 [P] Define `Source` sealed interface, `SubjectSource`, `StreamSource`, `ConsumerConfig`, `Decoder`, `EntityExtractor`, `Message`, `ChannelSpec` in `harness/channel.go` (per public-api.md "Channels" and contracts/stream-channel.md "Declaration")
- [X] T011 [P] Define `AwarenessContext` and `ReasoningContext` interfaces in `harness/context.go` — `AwarenessContext.State` only; `ReasoningContext.State`, `Publish`, `InFlight` (per public-api.md "Awareness"/"Reasoning" and research.md R-7); add a `// TODO` reminder for the compile-time test in T035
- [X] T012 [P] Define `StateRef` interface (`Get`, `Set`, `Update`) and the `ErrUnknownStateShape{Shape}` / `ErrCapExceeded{Shape, Count}` typed errors in `harness/state.go` (per public-api.md "State" and data-model.md "StateRef"/"Errors")
- [X] T013 [P] Define `ErrWhitelistViolation{Subject}` typed error in `harness/action.go` (per public-api.md "Errors" and FR-027)
- [X] T014 [P] Define `ErrSpecInvalid`, `ErrDuplicateStateShape`, `ErrConfigInvalid`, `ErrStreamNotFound`, `ErrConsumerIncompatible`, `ErrSubscriptionFailed` typed errors in `harness/errors.go` (per public-api.md "Errors"); each error implements `Error()` naming the offending value
- [X] T015 [P] Define `Option` type and `WithDrainWindow`, `WithLogger`, `WithSubjectPrefix`, `WithPlatformMode` constructors in `harness/options.go` with documented defaults (per public-api.md "Construction" and data-model.md "RuntimeOptions")
- [X] T016 Define the `Imp` handle struct + the `NewImp` signature stub in `harness/imp.go` (returns `(*Imp, error)`); leave runtime fields unexported and the body returning `errors.New("unimplemented")` for now (per public-api.md "Construction"/"Lifecycle")
- [X] T017 Implement spec validation logic in `internal/dispatch/validate.go` — checks FR-001/FR-002 (non-empty Name, non-nil Awareness/Reasoning, unique state shape names, positive caps, action de-duplication, exactly one source kind per channel); returns the typed errors from T012/T014
- [X] T017a [P] Unit test `internal/dispatch/validate_test.go` covering every validation branch from T017 — empty `Name`, nil `Awareness`, nil `Reasoning`, duplicate state-shape name (`*ErrDuplicateStateShape` with `Shape` populated), non-positive `Cap`, missing source kind, both source kinds set, empty channel name, empty `SubjectSource.Subject`, empty `StreamSource.Stream` / `FilterSubject`, and action-list de-duplication (duplicates accepted, observable single-membership semantics). Each case asserts the typed sentinel error and that `.Error()` names the offending field (FR-002, edge cases "Multiple state shapes share the same name" and "The same action subject appears on the whitelist more than once")
- [X] T018 Implement `subjects.Resolver` in `internal/subjects/resolver.go` covering both modes per `contracts/subject-resolution.md`: non-platform `prefix.declared`, platform `prefix.importerPK.declared`; expose `Resolve(declared string) string` and a constructor that fails with `ErrConfigInvalid` for missing prefix or missing importer PK in platform mode (FR-030/FR-031/FR-033)
- [X] T019 [P] Unit test `internal/subjects/resolver_test.go` covering both modes, missing-prefix error, missing-importer-pk error, and wildcard pass-through (per contracts/subject-resolution.md "Wildcard / pattern behavior")
- [X] T020 Implement bounded per-entity state registry in `internal/state/registry.go` — `sync.Map` per shape, atomic counter for cap, CAS-style allocation that returns `ErrCapExceeded` when full and never evicts (per data-model.md "StateShape" runtime invariants and research.md R-8)
- [X] T021 Implement concrete `StateRef` in `internal/state/ref.go` with per-slot mutex used by `Update` to serialize read-modify-write (per data-model.md "StateRef" Concurrency)
- [X] T022 [P] Unit test `internal/state/registry_test.go` exercising allocation up to cap, cap-exceeded on N+1, reads/writes on existing slots after cap, and unknown-shape error (FR-022/FR-023/FR-024/FR-025/FR-026)
- [X] T023 Implement metrics counter struct in `internal/observability/metrics.go` — atomics for each counter (`uint64`) plus `InflightReasoning` (`atomic.Int64`); expose `Snapshot()` returning `harness.Metrics` (per contracts/observability.md "Metrics snapshot" and data-model.md "Metrics")
- [X] T024 Implement `testutil/natstest/server.go` — starts an embedded `nats-server` per call with optional JetStream (temp dir + `t.Cleanup` removal); exposes `URL()`, `JetStream(t *testing.T)` helper that enables JetStream on demand (per research.md R-4)
- [X] T025 [P] Add a smoke test `testutil/natstest/server_test.go` verifying both core-only and JetStream-enabled modes start and shut down cleanly

**Checkpoint**: Foundation ready — public types compile, spec validation works, subject resolver passes its unit tests, state registry passes its cap tests, embedded NATS helper boots. User story implementation can now begin in parallel.

---

## Phase 3: User Story 1 - End-to-end imp dispatch (Priority: P1) 🎯 MVP

**Goal**: A developer declares a subject channel + awareness `Wake` + reasoning `Publish`, starts the harness, and a published message produces an action publish on the declared subject.

**Independent Test**: Construct a single-channel imp where awareness always returns `Wake` and reasoning publishes a known payload to one declared action subject. Run an embedded NATS server, publish on the channel subject, subscribe to the action subject, assert the action arrives within 1 s (SC-002).

### Tests for User Story 1

- [X] T026 [P] [US1] Integration test `integration/dispatch_test.go::TestEndToEndHappyPath` — publish on channel subject, expect action on whitelisted subject within 1 s (User Story 1 acceptance scenario 1, SC-002)
- [X] T027 [P] [US1] Integration test `integration/dispatch_test.go::TestDecodeFailureSkipsAwareness` — malformed payload increments `Metrics().DecodeFailures`, awareness not invoked, subsequent well-formed messages still dispatch (US1 acceptance scenario 2, FR-006)
- [X] T028 [P] [US1] Integration test `integration/dispatch_test.go::TestExtractionFailureSkipsAwareness` — extractor returning empty Entity increments `Metrics().ExtractionFailures`, awareness not invoked (US1 acceptance scenario 3, FR-007)
- [X] T028a [P] [US1] Integration test `integration/dispatch_test.go::TestAwarenessPanicRecovers` — first message triggers a deliberate panic inside the awareness function; second well-formed message still dispatches and produces the expected action publish; assert `Metrics().AwarenessPanics == 1`, no reasoning was queued for the panicking message, and dispatch goroutines are still alive (FR-015, edge case "Awareness panics during dispatch")

### Implementation for User Story 1

- [X] T029 [US1] Implement `harness.NewImp` body in `harness/imp.go` — runs spec validation (T017), constructs runtime options, builds `subjects.Resolver` (T018), constructs state registry (T020), constructs metrics (T023), wires the supplied `*nats.Conn`; returns the `*Imp` handle in `Created` state
- [X] T030 [US1] Implement subject channel subscription in `internal/dispatch/subject.go` — for each `SubjectSource` channel, `nc.Subscribe(resolver.Resolve(subject), handler)`; the handler builds a `harness.Message` and calls into the dispatch core (T031)
- [X] T031 [US1] Implement dispatch core in `internal/dispatch/dispatch.go` — `dispatch(channel, msg)` runs Decode → ExtractEntity → safeAwareness; on decode/extract error increments the matching metric counter and returns; on Wake verdict launches reasoning goroutine (T032)
- [X] T032 [US1] Implement reasoning launch in `internal/dispatch/reasoning.go` — increments `InflightReasoning` before goroutine start, decrements in deferred recover; `recover()` increments `ReasoningPanics`; returned errors increment `ReasoningErrors`; passes the imp's shutdown context as `ctx` to the reasoning fn (anchor for US6)
- [X] T033 [US1] Implement basic `Imp.Run(ctx)` in `harness/imp.go` — applies startup config validation (FR-033 deferred to US7), establishes all subject channels via T030, transitions `Created → Starting → Running`, blocks until `ctx` cancelled or `Shutdown` called; on cancellation calls a placeholder shutdown that simply unsubscribes (full drain ships in US8)
- [X] T034 [US1] Build the worked example `examples/echo/main.go` from `quickstart.md` — uses `WithSubjectPrefix("tenant.demo")`; verifies the imp builds against the public API surface as it lands

**Checkpoint**: User Story 1 fully functional — the echo imp builds, runs against `nats-server`, and the integration tests at T026-T028 pass. MVP boundary.

---

## Phase 4: User Story 2 - Awareness verdicts produce the right downstream behavior (Priority: P1)

**Goal**: `Ignore` does nothing further; `Note(payload)` invokes the imp's `OnNote` hook (no reasoning); `Wake(reason, entity)` queues reasoning asynchronously and dispatch returns before reasoning completes.

**Independent Test**: Per spec User Story 2 — three minimal channels each with a different verdict; assert the observable side effects per verdict and that channel dispatch returns before reasoning finishes.

### Tests for User Story 2

- [X] T035 [P] [US2] Integration test `integration/verdict_test.go::TestIgnoreVerdict` — no reasoning queued, no Note record, `Metrics().IgnoredVerdicts` increments (FR-011, US2 acceptance 1)
- [X] T036 [P] [US2] Integration test `integration/verdict_test.go::TestNoteVerdict` — `OnNote` hook receives the payload, `Metrics().NotesDelivered` increments, reasoning not invoked (FR-012, US2 acceptance 2)
- [X] T037 [P] [US2] Integration test `integration/verdict_test.go::TestWakeVerdictAsync` — reasoning runs with the supplied reason+entity, `Metrics().WakesDispatched` increments, channel dispatch returns before reasoning's signal is observed (FR-013, US2 acceptance 3)

### Implementation for User Story 2

- [X] T038 [US2] Add verdict pattern matching in `internal/dispatch/dispatch.go` — switch on the unexported verdict discriminator and route to: no-op (Ignore), `OnNote` invocation (Note), reasoning launch (Wake); each branch increments the matching metric (per contracts/observability.md "Metrics snapshot")
- [X] T039 [US2] Wire `OnNote` hook invocation with panic recovery in `internal/dispatch/note.go` — recovered panic increments `AwarenessPanics` and is logged at ERROR (per contracts/observability.md "Note hook" constraints); nil `OnNote` simply increments the counter and drops the payload

**Checkpoint**: Verdict semantics enforced exactly. Stories 1 and 2 both work; the energy gradient (cheap awareness, async reasoning) is observable.

---

## Phase 5: User Story 3 - The awareness/reasoning boundary is structural (Priority: P1)

**Goal**: A developer cannot publish from awareness — the method does not exist on the awareness context type (compile error). At runtime, off-whitelist publishes from reasoning return `ErrWhitelistViolation` before reaching NATS, with the imp continuing to run.

**Independent Test**: Compile-time check that `awareness.Publish(...)` does not compile (SC-006). Runtime check that publishing to a non-whitelisted subject returns the typed error and no message reaches the substrate (SC-005).

### Tests for User Story 3

- [X] T040 [P] [US3] Build-tagged compile-time test file `integration/compiletest/awareness_no_publish.go` — references `awareness.Publish` under a build tag; presence of compile failure is the assertion (SC-006, public-api.md "Compile-time guarantees" #1). Document the expected `go vet` / `go build` invocation in `integration/compiletest/README.md`
- [X] T041 [P] [US3] Integration test `integration/boundary_test.go::TestOffWhitelistPublishRejected` — reasoning calls `Publish` on a subject not in `Actions`; assert `errors.As` to `*harness.ErrWhitelistViolation`, `Subject` matches, no message reaches a wildcard subscriber on the substrate, `Metrics().WhitelistViolations` increments (FR-027, SC-005, US3 acceptance 2)
- [X] T042 [P] [US3] Integration test `integration/boundary_test.go::TestWhitelistedPublishSucceeds` — reasoning calls `Publish` on a whitelisted subject; subscriber observes the message on the resolved subject; `Publish` returns nil (US3 acceptance 3)

### Implementation for User Story 3

- [X] T043 [US3] Implement concrete awareness context in `internal/dispatch/context_awareness.go` — wraps the state registry; only exports `State`. Verify by inspection that no `Publish` method exists on the type
- [X] T044 [US3] Implement concrete reasoning context in `internal/dispatch/context_reasoning.go` — `State` (delegates to registry), `Publish` (whitelist-checks then resolves then publishes via `*nats.Conn`), `InFlight` (reads the atomic gauge)
- [X] T045 [US3] Implement whitelist as a `map[string]struct{}` built at `NewImp` time in `internal/dispatch/whitelist.go` — `Check(subject) error` returns `ErrWhitelistViolation` for non-members; whitelist is on declared (pre-resolution) subjects (per research.md R-12 and contracts/subject-resolution.md "Symmetry guarantees" #3)
- [X] T046 [US3] Wire `Publish` flow: whitelist check → subject resolution → `nc.Publish(resolved, payload)`; on whitelist violation, increment `Metrics().WhitelistViolations`, log at WARN, return without touching NATS

**Checkpoint**: Constitutional guarantee enforced. Compile-time absence (SC-006) and runtime call-site rejection (SC-005) both hold. Stories 1, 2, and 3 all work.

---

## Phase 6: User Story 4 - Stream channel ingestion with declared durability (Priority: P1)

**Goal**: A developer declares a stream channel referencing a JetStream stream + filter subject + optional durable consumer name. Dispatch behavior is identical to subject channels. Durables survive restart; ephemerals are torn down on shutdown. Stream-not-found and incompatible-consumer cases fail startup with clear errors.

**Independent Test**: Per spec User Story 4 — durable consumer survives restart and resumes from durable position; ephemeral consumer is removed on shutdown.

### Tests for User Story 4

- [X] T047 [P] [US4] Integration test `integration/stream_channel_test.go::TestStreamChannelDurableHappyPath` — declares a stream channel with `Durable: "echo-orders"`, publishes to the stream's source subject, asserts the action arrives; restarts the harness; publishes again; asserts the consumer resumes without redelivering acked messages (US4 acceptance 1)
- [X] T048 [P] [US4] Integration test `integration/stream_channel_test.go::TestEphemeralConsumerLifecycle` — declares a stream channel with empty `Durable`, asserts the ephemeral consumer is created at startup and removed on clean shutdown (queryable via `jetstream.Stream.Consumer`) (FR-005b, US4 acceptance 2)
- [X] T049 [P] [US4] Integration test `integration/stream_channel_test.go::TestStreamNotFound` — declares a stream channel referencing a stream that does not exist; assert `Run` returns `*harness.ErrStreamNotFound` and no subscriptions remain (FR-005c, US4 acceptance 3)
- [X] T050 [P] [US4] Integration test `integration/stream_channel_test.go::TestConsumerIncompatible` — pre-creates a durable consumer with a different filter subject; declares a stream channel with that durable name; assert `Run` returns `*harness.ErrConsumerIncompatible` with the diff summary (FR-005a, US4 acceptance 4)
- [X] T051 [P] [US4] Integration test `integration/stream_channel_test.go::TestAckAtAwarenessCompletion` — reasoning that takes 500 ms; assert the underlying message is acked (consumer pending count drops) before reasoning completes (FR-008a, US4 acceptance 5)
- [X] T052 [P] [US4] Integration test `integration/stream_channel_test.go::TestNakOnFailures` — three sub-cases: decode failure, extraction failure, awareness panic; each NAKs the message; consumer redelivers up to `MaxDeliver`; `Metrics().NakTotal` increments per NAK (FR-008a, US4 acceptance 6)

### Implementation for User Story 4

- [X] T053 [US4] Implement consumer resolution in `internal/stream/consumer.go` — for durable name: lookup → bind if compatible → create if absent → `ErrConsumerIncompatible` if incompatible; for empty durable: create ephemeral with generated name and remember it for teardown (per contracts/stream-channel.md "Startup behavior")
- [X] T054 [US4] Implement compatibility check in `internal/stream/compat.go` — compares declared `ConsumerConfig` against existing consumer info per the rules table in `contracts/stream-channel.md` "Compatibility check"; returns `ErrConsumerIncompatible{Consumer, Diff}` with a human-readable diff
- [X] T055 [US4] Implement consume loop in `internal/stream/consume.go` — uses `jetstream.Consumer.Consume` push-style callback; per-message flow exactly as specified in `contracts/stream-channel.md` "Per-message dispatch and ack timing": Decode → Extract → Awareness → ACK → branch on verdict; NAK on any pre-ack failure
- [X] T056 [US4] Wire stream channels into `Imp.Run` startup in `internal/lifecycle/start.go` — for each `StreamSource` channel: stream existence check (returns `ErrStreamNotFound`), consumer resolution (T053), start consume loop (T055); record consumer name on `ChannelState` for shutdown cleanup
- [X] T057 [US4] Implement ephemeral consumer teardown in `internal/stream/teardown.go` — invoked from shutdown; deletes consumers with empty `Durable`; logs (does not fail) on delete error; durables left in place per contracts/stream-channel.md "Shutdown teardown"

**Checkpoint**: All four P1 user stories functional. The harness now satisfies the constitutional commitments (energy gradient, awareness/reasoning boundary, action whitelist) and supports both subject and stream channels with identical dispatch semantics.

---

## Phase 7: User Story 5 - Per-entity local memory with bounded capacity (Priority: P2)

**Goal**: Per-entity state instances allocated lazily on first reference, returned consistently on subsequent references, capped per shape with a typed cap-exceeded error. No silent eviction. Reads/writes on existing slots succeed after cap is reached. Unknown shape names return a typed error.

**Independent Test**: Drive `N` distinct entities through awareness with cap `N`; drive entity `N+1` and verify cap-exceeded error; verify reads/writes on the original `N` entities still succeed.

### Tests for User Story 5

- [X] T058 [P] [US5] Integration test `integration/memory_test.go::TestPerEntityStateConsistency` — drives entities 1..N through awareness, each `State("counter", e)` returns a stable instance across calls; subsequent reads see prior writes (FR-022, FR-023, US5 acceptance 1)
- [X] T059 [P] [US5] Integration test `integration/memory_test.go::TestCapExceededOnNewEntity` — with cap N reached, allocating entity N+1 returns `*harness.ErrCapExceeded` naming the shape and current count; `Metrics()` shows no eviction occurred (FR-024, US5 acceptance 2)
- [X] T060 [P] [US5] Integration test `integration/memory_test.go::TestExistingSlotsAfterCap` — after the cap-exceeded error, `Set` and `Get` and `Update` on the existing N entities all succeed (FR-025, US5 acceptance 3)
- [X] T061 [P] [US5] Integration test `integration/memory_test.go::TestUnknownStateShapeError` — calling `State("not-declared", entity)` returns `*harness.ErrUnknownStateShape` (FR-026, edge case "unknown state shape")
- [X] T062 [P] [US5] Integration test `integration/memory_test.go::TestConcurrentSameEntitySerialized` — two channels triggering awareness for the same entity concurrently, each performing `Update`; assert all updates apply (no lost write) (edge case "Two awareness calls for the same entity arrive concurrently")

### Implementation for User Story 5

- [X] T063 [US5] Wire `AwarenessContext.State` and `ReasoningContext.State` calls through to the registry built in T020 — both contexts share the same registry; the registry returns the typed errors directly
- [X] T064 [US5] Verify `StateRef.Update` (T021) serializes per slot under a slot-local mutex; document the cross-shape ordering caveat (no global lock) in the file comment of `internal/state/ref.go`

**Checkpoint**: Local memory works as specified. Stories 1-5 functional. Cap fails loudly, never silently — preserving the `eviction belongs to sleep/wake` boundary.

---

## Phase 8: User Story 6 - Reasoning runs concurrently and never blocks awareness (Priority: P2)

**Goal**: Reasoning invocations for distinct entities run concurrently. A held reasoning never blocks awareness for new messages. On shutdown, in-flight reasoning is given a configured drain window before the harness returns.

**Independent Test**: Per spec User Story 6 — reasoning blocks on a controllable signal; new messages keep dispatching; both reasoning invocations are observably running before either is released.

### Tests for User Story 6

- [X] T065 [P] [US6] Integration test `integration/concurrency_test.go::TestConcurrentReasoningDistinctEntities` — two messages for E1 and E2 produce two reasoning goroutines that overlap (verified via shared sync primitive); `Metrics().InflightReasoning` reaches 2 before either is released (FR-018, US6 acceptance 1, SC-004)
- [X] T066 [P] [US6] Integration test `integration/concurrency_test.go::TestAwarenessNotBlockedByHeldReasoning` — reasoning held under deliberate block; subsequent messages on the same channel still dispatch awareness within 50 ms additional latency vs. baseline (FR-020, US6 acceptance 2, SC-003)
- [X] T067 [P] [US6] Integration test `integration/concurrency_test.go::TestShutdownDrainWindow` — start two slow reasoning invocations; call `Shutdown` with `WithDrainWindow(2*time.Second)`; assert (a) reasoning context is cancelled, (b) shutdown returns within `2*time.Second + small ε`, (c) `Metrics().InflightReasoning` is reported truthfully even past the deadline (FR-036, US6 acceptance 3, SC-009)
- [X] T067a [P] [US6] Integration test `integration/concurrency_test.go::TestReasoningPanicIsolation` — two messages produce reasoning invocations for entities `E1` (panics) and `E2` (publishes its action); assert (a) `E2`'s action arrives normally, (b) `Metrics().ReasoningPanics == 1`, (c) `Metrics().InflightReasoning` returns to zero, (d) a third subsequent message for any entity dispatches and reasons normally — proving other invocations and entities are unaffected (FR-021, edge case "Reasoning panics or returns an error")

### Implementation for User Story 6

- [X] T068 [US6] Verify `internal/dispatch/dispatch.go` Wake branch never awaits reasoning completion — add an explicit `// returns before reasoning runs` comment and a benchmark `BenchmarkDispatchWakeReturns` in `internal/dispatch/dispatch_bench_test.go` showing the dispatch path returns in microseconds regardless of reasoning latency
- [X] T069 [US6] Wire shutdown context cancellation through to in-flight reasoning in `internal/lifecycle/shutdown.go` — `Shutdown` cancels the reasoning-side context first, then waits up to `drainWindow` on the reasoning `sync.WaitGroup`, then returns regardless (per research.md R-14)

**Checkpoint**: The concurrency invariant (FR-018, FR-020, FR-021, SC-003, SC-004) holds. Stories 1-6 functional.

---

## Phase 9: User Story 7 - Same code, both deployment modes (Priority: P2)

**Goal**: The same imp source compiles and runs in non-platform mode (prefix only) and platform mode (prefix + importer account public key as a fixed segment). Endpoints, dispatch behavior, contracts, and developer-facing API are identical across modes.

**Independent Test**: Run the same imp twice — once with `WithSubjectPrefix("tenantA.imps.demo")` (non-platform), once with `WithSubjectPrefix("platform")` + `WithPlatformMode("ACCOUNT_PK")` — and assert the resolved channel and action subjects match the rules in `contracts/subject-resolution.md`.

### Tests for User Story 7

- [X] T070 [P] [US7] Integration test `integration/modes_test.go::TestNonPlatformModeResolution` — channel declared as `messages.in`, action `actions.out`, prefix `tenantA.imps.demo`; assert subscription on `tenantA.imps.demo.messages.in`, action publish on `tenantA.imps.demo.actions.out` (US7 acceptance 1, FR-030)
- [X] T071 [P] [US7] Integration test `integration/modes_test.go::TestPlatformModeResolution` — same imp source as T070, switched to platform mode with importer pk `ACCOUNT_PK`; assert subscription on `platform.ACCOUNT_PK.messages.in`, action publish on `platform.ACCOUNT_PK.actions.out`; assert imp source bytes are byte-identical between the two runs (US7 acceptance 2, FR-031, SC-008)
- [X] T072 [P] [US7] Integration test `integration/modes_test.go::TestPlatformModeMissingImporterPK` — `WithPlatformMode("")` (or no `WithPlatformMode` set after enabling platform mode); assert `Run` returns `*harness.ErrConfigInvalid{Field: "importer_account_pk"}` (FR-033, US7 acceptance 3, edge case)
- [X] T073 [P] [US7] Integration test `integration/modes_test.go::TestNonPlatformModeMissingPrefix` — `Run` without `WithSubjectPrefix`; assert `*harness.ErrConfigInvalid{Field: "prefix"}` (per contracts/subject-resolution.md "Failure modes")

### Implementation for User Story 7

- [X] T074 [US7] Wire `WithPlatformMode` and `WithSubjectPrefix` into the resolver constructor (T018) at `Run` time — fold runtimeOptions into the resolver; both channels (T030, T056) and reasoning publishes (T046) consume the same `Resolve` call
- [X] T075 [US7] Add startup config validation gate in `internal/lifecycle/start.go` — runs before any subscription is established; returns `ErrConfigInvalid` for non-platform-mode-without-prefix and platform-mode-without-importer-pk; ensures FR-035 (no dangling subscriptions on startup failure) holds

**Checkpoint**: One source builds and runs in both modes (SC-008). Stories 1-7 functional.

---

## Phase 10: User Story 8 - Clean lifecycle (Priority: P3)

**Goal**: Startup establishes every declared subscription and registers identity, or aborts cleanly with no dangling subscriptions. Shutdown stops accepting new messages, drains in-flight reasoning within the configured window, unsubscribes cleanly, and returns within the drain deadline.

**Independent Test**: Per spec User Story 8 — start the harness, observe subscriptions, call `Shutdown` while two reasoning invocations are in flight, assert subscriptions removed and shutdown returns within the bounded window.

### Tests for User Story 8

- [X] T076 [P] [US8] Integration test `integration/lifecycle_test.go::TestStartupRegistersSubscriptions` — after `Run` returns ready (use a poll on `Identity()` or a readiness signal), assert each declared channel has a corresponding subscription on the resolved subject and `Identity()` returns the expected `(Name, Version, SubjectPrefix)` (FR-003, FR-034, US8 acceptance 1)
- [X] T077 [P] [US8] Integration test `integration/lifecycle_test.go::TestStartupFailureNoLeaks` — force a startup failure (e.g., declared action subject is empty after the spec validation surface, or a stream-source references a missing stream after one subject channel has already been wired); assert no NATS subscriptions remain registered and no goroutines remain running (FR-035, US8 acceptance 2)
- [X] T078 [P] [US8] Integration test `integration/lifecycle_test.go::TestShutdownDrainBoundedReturn` — two slow reasoning invocations in flight; `Shutdown` with drain window `D`; assert (a) subscriptions removed within ε of call, (b) shutdown returns no later than `D + small ε`, (c) goroutine count returns to baseline (US8 acceptance 3, SC-009)
- [X] T079 [P] [US8] Integration test `integration/lifecycle_test.go::TestIdentityAcrossStates` — `Identity()` returns valid values during `Running`, `Draining`, and `Stopped`; partial values (Name, Version) available after `Failed` startup (per data-model.md "Lifecycle states")

### Implementation for User Story 8

- [X] T080 [US8] Implement startup phase with rollback in `internal/lifecycle/start.go` — establishes subscriptions in declared order; on any failure, unsubscribes everything established so far and returns the typed error; transitions `Created → Starting → Running` on success or `→ Failed` on error
- [X] T081 [US8] Implement full `Imp.Shutdown` in `internal/lifecycle/shutdown.go` — idempotent (subsequent calls return nil); cancels reasoning context; unsubscribes all subject channels; stops all stream consume loops; tears down ephemeral consumers (T057); waits up to `drainWindow` on the WaitGroup; returns within `drainWindow + ε`
- [X] T082 [US8] Implement `Imp.Identity` accessor in `harness/imp.go` — reads from runtime state populated at startup; returns `ImpIdentity{Name, Version, SubjectPrefix}`; safe to call concurrently and from any lifecycle state
- [X] T083 [US8] Wire lifecycle states (`Created`/`Starting`/`Running`/`Draining`/`Stopped`/`Failed`) in `internal/lifecycle/state.go` — atomic state machine; one-way transitions per data-model.md "Lifecycle states"; `Run` and `Shutdown` consult and update it

**Checkpoint**: All eight user stories functional. The harness starts cleanly, fails cleanly, and shuts down within the drain deadline.

---

## Phase 11: Polish & Cross-Cutting Concerns

**Purpose**: Finalize the surface, run the full quality gate, and validate against the spec's success criteria.

- [X] T084 [P] Verify `examples/echo/main.go` (T034) still builds and runs end-to-end against an embedded `nats-server`; add `examples/echo/main_test.go` mirroring the integration-test pattern from `quickstart.md` "Testing your imp"
- [X] T085 [P] Add a benchmark `internal/dispatch/dispatch_bench_test.go::BenchmarkDispatchOverhead` measuring per-message dispatch overhead with N=0, N=100, N=1000 in-flight reasoning and N=10, N=1000 tracked entities; document that growth is sub-linear in both dimensions (SC-010)
- [X] T086 [P] Add a stress test `integration/throughput_test.go::TestSustainedAwarenessUnderLoad` — sustained channel publishes for thousands of entities while reasoning remains in flight; assert no observable awareness backpressure (SC-011)
- [X] T087 [P] Wire structured-logger events listed in `contracts/observability.md` "Logger" — start/ready/shutdown lifecycle, decode/extract/awareness/reasoning failures, whitelist violations, stream channel events; verify by capturing slog output in a buffer in unit tests
- [X] T088 Run `make fmt && make tidy && make test && make lint` from repo root and resolve every issue (CLAUDE.md mandate: "ALL hook issues are BLOCKING")
- [X] T089 [P] Update `harness/doc.go` to mention examples + the spec link, and add a one-paragraph package overview matching the public-api.md surface
- [X] T090 Walk through `quickstart.md` end-to-end manually (or via a script) and confirm every step works: build the echo imp, switch to platform mode, add per-entity state, add a stream channel, run the integration test pattern. Fix any drift discovered

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies; can start immediately.
- **Phase 2 (Foundational)**: Depends on Phase 1. Blocks all user story phases.
- **Phase 3 (US1)**: Depends on Phase 2. MVP gate.
- **Phase 4 (US2)**: Depends on Phase 2 and the dispatch core (T031) from Phase 3.
- **Phase 5 (US3)**: Depends on Phase 2 and the reasoning context wiring (T032/T044) — can begin in parallel with Phase 4.
- **Phase 6 (US4)**: Depends on Phase 2 and the dispatch core (T031); the JetStream piece is independent of US2/US3 and can start in parallel.
- **Phase 7 (US5)**: Depends on Phase 2 and the registry/contexts (T020/T021/T043/T044). Independent of US2/US3/US4 implementations, so can run in parallel after US3 lands the contexts (T043, T044).
- **Phase 8 (US6)**: Depends on Phase 2 and the reasoning launch (T032). Can run in parallel with US4/US5.
- **Phase 9 (US7)**: Depends on Phase 2 (resolver T018) and the publish path (T046). Can run in parallel with US4/US5/US6.
- **Phase 10 (US8)**: Depends on Phase 2 and at least one channel implementation (T030 or T056). Best done after Phase 6 (drain semantics already partially live).
- **Phase 11 (Polish)**: Depends on all desired user story phases.

### User Story Dependencies (recap from spec)

- **US1 (P1)**: Foundational only.
- **US2 (P1)**: Foundational + dispatch core; otherwise independent of US3/US4.
- **US3 (P1)**: Foundational + reasoning context; independent of US2/US4 at the test level.
- **US4 (P1)**: Foundational + dispatch core; the JetStream binding is its own internal package.
- **US5 (P2)**: Foundational + state registry + contexts.
- **US6 (P2)**: Foundational + reasoning launch + shutdown wiring.
- **US7 (P2)**: Foundational + resolver + publish path.
- **US8 (P3)**: Foundational + at least one channel + reasoning lifecycle.

### Within Each User Story

- Tests are written first and watched fail before the implementation tasks for that story land.
- Models/types (Phase 2) before services (per-story implementations).
- Per-story implementation tasks in the listed order; downstream work that crosses files is non-parallel within a single phase unless explicitly marked [P].
- Each story's checkpoint must hold before moving to the next priority.

### Parallel Opportunities

- All Phase 1 setup tasks marked [P] (T002-T006) run in parallel after T001.
- All Phase 2 type-definition tasks marked [P] (T007-T015) run in parallel; the validator (T017) waits on them; the registry (T020) and resolver (T018) and metrics (T023) run in parallel after the types land.
- All integration tests for a user story marked [P] (e.g., T026/T027/T028 for US1, T035-T037 for US2) run in parallel — they are in the same file but write independent test functions.
- Stories US4/US5/US6/US7 can be picked up by separate developers in parallel after Phase 5 (US3) lands the reasoning context concrete type.

---

## Parallel Example: User Story 1

```bash
# Launch all US1 integration tests together (different test functions, same file is fine):
Task: "Integration test integration/dispatch_test.go::TestEndToEndHappyPath"
Task: "Integration test integration/dispatch_test.go::TestDecodeFailureSkipsAwareness"
Task: "Integration test integration/dispatch_test.go::TestExtractionFailureSkipsAwareness"

# After T029 (NewImp) lands, the implementation chain T030 → T031 → T032 → T033 → T034
# is sequential because each depends on the previous file's surface.
```

## Parallel Example: Phase 2 Foundational

```bash
# All public-type definitions are different files, no inter-dependencies:
Task: "Define Entity in harness/spec.go"
Task: "Define ImpSpec/ImpIdentity/Metrics/StateShape in harness/spec.go"  # same file, sequential
Task: "Define Verdict in harness/verdict.go"
Task: "Define channel types in harness/channel.go"
Task: "Define context interfaces in harness/context.go"
Task: "Define StateRef + state errors in harness/state.go"
Task: "Define ErrWhitelistViolation in harness/action.go"
Task: "Define remaining errors in harness/errors.go"
Task: "Define Option type in harness/options.go"

# Internal infrastructure runs in parallel after the types land:
Task: "internal/subjects/resolver.go (+ resolver_test.go)"
Task: "internal/state/registry.go + ref.go (+ registry_test.go)"
Task: "internal/observability/metrics.go"
Task: "testutil/natstest/server.go (+ server_test.go)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup
2. Phase 2: Foundational
3. Phase 3: User Story 1 — channel → awareness → reasoning → action
4. **STOP and VALIDATE**: Run the echo example end-to-end, watch the integration tests pass.
5. Demo if ready.

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. US1 → MVP demo
3. US2 (verdicts) + US3 (boundary) — both P1, can land in either order; both must land before public preview
4. US4 (stream channels) — final P1; the harness now matches the spec's "first-class subject and stream channels" claim
5. US5 (memory) → demo
6. US6 (concurrency) → demo
7. US7 (modes) → demo
8. US8 (lifecycle) → demo
9. Polish → release-ready

### Parallel Team Strategy

After Phase 2 lands:

- Developer A: US1 (MVP) → US2 (verdicts) → US6 (concurrency)
- Developer B: US3 (boundary) → US5 (memory) → US8 (lifecycle)
- Developer C: US4 (stream channels) → US7 (modes) → polish

Stories complete and integrate independently; the integration test files do not overlap.

---

## Notes

- [P] tasks live in different files (or are independent test functions in the same file) and have no incomplete dependencies.
- [Story] label maps each task to a specific user story for traceability.
- Each user story is independently testable against an embedded `nats-server` via `testutil/natstest`.
- Verify integration tests fail before implementing — they describe the spec's acceptance scenarios literally.
- Run `make fmt && make test && make lint` after each phase (CLAUDE.md reality checkpoint).
- Avoid: vague tasks (e.g., "implement reasoning") without a file path; cross-story dependencies that break independent testability; introducing capability clients, soulstream, persistence, KV channels, audit emission, queue groups, or per-entity reasoning serialization (FR-NS-1…FR-NS-4 explicitly defer all of these).
