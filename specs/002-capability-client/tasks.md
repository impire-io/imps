---
description: "Task list for the Request/Reply Surface feature (002-capability-client)"
---

# Tasks: Request/Reply Surface

**Input**: Design documents from `/specs/002-capability-client/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/request-reply.md

**Tests**: Test tasks ARE included. The feature spec lists explicit Independent-Test procedures for every user story, plus Success Criteria SC-101 … SC-107 that are only verifiable via integration tests. Compile-time guarantees (US-4) are verified by build-tagged files.

**Organization**: Tasks are grouped by user story (US-1 … US-7 + SC-107) to enable independent implementation and testing. User-story order below follows the dependency chain from `plan.md` § "Phase 2: task strategy", which differs from numeric spec order: US-1 → US-3 → US-4 → US-2 → US-5 → US-6 → US-7 → SC-107.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US3, US4, US2, US5, US6, US7)
- Include exact file paths in descriptions

## Path Conventions

- Single Go module at repo root.
- Public package: `harness/`.
- Black-box integration tests: `integration/`.
- Build-tagged compile-time assertions: `integration/compiletest/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm baseline tooling and pre-feature build cleanliness. No new project scaffolding is needed — this feature layers on the existing `001-harness-core` module.

- [X] T001 Confirm baseline build is green: run `make fmt && make test && make lint` from the repo root and capture the clean baseline before adding any new files.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add the option, error, metrics, and helper plumbing that every user story consumes. Without these, no user-story phase can compile.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 [P] Add typed error sentinels `ErrNoResponders` and `ErrRequestTimeout` (with `Error()` and `Unwrap() error` returning `context.DeadlineExceeded` on `ErrRequestTimeout`) to `harness/errors.go` per data-model.md "Error types" and contract § Errors (FR-118).
- [X] T003 [P] Add the four new counter fields (`requestCalls`, `requestManyCalls`, `requestNoResponders`, `requestTimeouts`) to the unexported metrics container in `harness/metrics_internal.go`, and extend `snapshot()` to read them (FR-120, R-111).
- [X] T004 [P] Extend the public `Metrics` snapshot struct in `harness/spec.go` with `RequestCalls`, `RequestManyCalls`, `RequestNoResponders`, `RequestTimeouts` (FR-120, contract § Observability).
- [X] T005 Add the two new construction options `WithDefaultRequestTimeout(d time.Duration)` and `WithDefaultRequestManyWindow(d time.Duration)` plus the matching `runtimeOptions` fields and defaults (5 s / 1 s) to `harness/options.go` (FR-116, R-106). Apply defaults in `defaultRuntimeOptions()`.
- [X] T006 Add validation in `bootRuntime` (or wherever 001's option validation lives — see `harness/imp.go`) so non-positive `defaultRequestTimeout` or `defaultRequestManyWindow` return `*ErrConfigInvalid{Field: "default_request_timeout"|"default_request_many_window", Reason: "non-positive"}` (FR-116, FR-119).
- [X] T007 [P] Define public per-call option types and constructors in `harness/request.go` (new file): `RequestOption`, `RequestManyOption`, plus internal `requestOptions` / `requestManyOptions`, and the constructors `WithRequestTimeout`, `WithRequestManyWindow`, `WithRequestManyMax`. Implement the "non-positive is silent no-op" rule for timeouts/windows and "n <= 0 means no cap" for max (FR-117, R-106, data-model.md § RequestOption/RequestManyOption).
- [X] T008 In `harness/request.go`, implement the shared dispatch helpers (R-103, R-104, data-model.md § Dispatch helpers):
  - `requestSingle(ctx, nc, m, log, defaultTimeout, subject, payload, opts) ([]byte, error)` — increment `RequestCalls`, compute effective timeout, derive `context.WithTimeout`, call `nc.RequestWithContext` on the literal subject, translate errors (`nats.ErrNoResponders` → `*ErrNoResponders`, derived-context `context.DeadlineExceeded` → `*ErrRequestTimeout`, caller cancellation → returned error preserving `errors.Is(err, context.Canceled)`, other substrate errors passed through), and increment the matching failure counter.
  - `requestMany(ctx, nc, m, log, defaultWindow, subject, payload, opts) ([][]byte, error)` — increment `RequestManyCalls`, compute effective window and cap, create inbox via `nc.NewRespInbox()`, `nc.ChanSubscribe` with buffer `max(k, 64)`, `nc.PublishMsg{Subject, Reply: inbox, Data: payload}`, collect in `select` over (replyCh, `time.After(window)`, `ctx.Done()`), `defer sub.Unsubscribe()` on every return path (FR-113), translate caller cancellation and substrate publish refusal per R-104.
- [X] T009 Extend the awareness concrete type in `harness/context_awareness.go` (and `harness/context.go` interface) with the `Request(ctx, subject, payload, opts...) ([]byte, error)` method that delegates to `requestSingle`. Add `conn *nats.Conn` and `defaultRequestTimeout time.Duration` to the concrete struct; populate them in `bootRuntime`/wherever `awarenessCtx` is constructed today (data-model.md § Awareness/Reasoning context concrete types).
- [X] T010 Extend the reasoning concrete type in `harness/context_reasoning.go` (and `harness/context.go` interface) with `Request(ctx, subject, payload, opts...) ([]byte, error)` (delegates to `requestSingle`) and `RequestMany(ctx, subject, payload, opts...) ([][]byte, error)` (delegates to `requestMany`). Add `conn`, `defaultRequestTimeout`, and `defaultRequestManyWindow` fields; populate in construction. `Publish` and `Conn()` already exist from the 001 v2.2.0 cleanup — leave them untouched.
- [X] T011 [P] Update package godoc in `harness/doc.go` to describe the outbound surface: the awareness `Request`-only / reasoning `Request`+`RequestMany`+`Publish`+`Conn` split, the two typed errors, and the verbatim-subject discipline. No marketing prose — short paragraphs matching the contract.

**Checkpoint**: Foundation ready — the harness compiles with the new surface, defaults validate at `Run`, dispatch helpers exist but are exercised only by user-story phases below.

---

## Phase 3: User Story 1 — Reasoning reaches a service via Request (Priority: P1) 🎯 MVP

**Goal**: A reasoning function can call `r.Request(ctx, subject, payload)` and receive the reply bytes (or one of the two typed errors). This is the foundational round-trip and the MVP slice.

**Independent Test**: Embedded NATS, plain `nc.Subscribe("knowledge.recall", echoResponder)`, an imp whose reasoning calls `r.Request(ctx, "knowledge.recall", []byte("ping"))`, drive one message through — assert reply equals `[]byte("ping")` and no error (spec.md US-1 Independent Test).

### Tests for User Story 1

- [X] T012 [US1] Integration test `TestRequest_Reasoning_HappyPath` in `integration/request_test.go` covering US-1 acceptance scenarios 1 and 2: subscribe an echo handler on `knowledge.recall`, build an imp whose reasoning calls `r.Request`, drive a message, assert (a) the reply matches the echoed payload, (b) the call respects `WithRequestTimeout(d)` when supplied, (c) `Metrics.RequestCalls` incremented by exactly one.

### Implementation for User Story 1

All implementation needed for US-1 was completed in Phase 2 (T008/T010). T012 only verifies behavior; no new production code lands in this phase.

**Checkpoint**: Reasoning `Request` happy-path delivers MVP. Stop and validate via `make fmt && make test && make lint`.

---

## Phase 4: User Story 3 — Awareness reaches a service via Request (Priority: P1)

**Goal**: An awareness function can call `a.Request(ctx, subject, payload, opts...)` to issue a single bounded round-trip during dispatch and use the reply in its verdict.

**Independent Test**: Embedded NATS, plain `nc.Subscribe("embed.short", deterministicResponder)`, an imp whose awareness calls `a.Request` and chooses `Wake` vs `Ignore` from the reply — assert the verdict reflects the response, within the per-call timeout (spec.md US-3 Independent Test).

> Sequenced before US-2 per `plan.md` because awareness's `Request` shares zero code with `RequestMany`; landing it next exercises the awareness wiring while the helper is still small.

### Tests for User Story 3

- [X] T013 [US3] Integration test `TestRequest_Awareness_HappyPath` in `integration/request_test.go` (append to the file from T012) covering US-3 AS-1: subscribe a deterministic transformer on `embed.short`, build an imp whose awareness calls `a.Request(...)` and uses the reply to choose `Wake` vs `Ignore`, drive one message, assert the verdict matches the responder's reply within the configured awareness-call timeout.
- [X] T014 [US3] Integration test `TestRequest_Awareness_TimeoutInVerdict` in the same file covering US-3 AS-2: subscribe a delayed responder (e.g., 200 ms) on `embed.short`, awareness calls `a.Request(..., WithRequestTimeout(50*time.Millisecond))`, assert `*ErrRequestTimeout` and that awareness still yields a verdict (e.g., a `Note` recording the degraded state).

### Implementation for User Story 3

Implementation already landed in Phase 2 (T009). No additional production code.

**Checkpoint**: Awareness `Request` works alongside reasoning `Request`. Re-run `make fmt && make test && make lint`.

---

## Phase 5: User Story 4 — Awareness cannot RequestMany, Publish, or Conn (Priority: P1)

**Goal**: The energy gradient is structurally enforced: build-tagged files prove `AwarenessContext` exposes neither `RequestMany` nor `Publish` nor `Conn` (SC-104, FR-102, FR-103, contract § Compile-time guarantees).

**Independent Test**: `go vet -tags=awareness_requestmany_must_fail ./integration/compiletest/...` MUST exit non-zero. Same for `awareness_publish_must_fail` (already covered from 001) and `awareness_conn_must_fail`. A reasoning-side smoke test confirms reasoning *can* invoke `RequestMany` and `Publish` (spec.md US-4 (c)).

### Tests for User Story 4

- [X] T015 [P] [US4] Create `integration/compiletest/awareness_no_requestmany.go` with `//go:build awareness_requestmany_must_fail`, mirroring the structure of `awareness_no_publish.go`: a function with parameter `a harness.AwarenessContext` whose body calls `a.RequestMany(...)`. Building under that tag MUST fail. Include a brief comment naming the tag and the assertion (R-109, SC-104).
- [X] T016 [P] [US4] Create `integration/compiletest/awareness_no_conn.go` with `//go:build awareness_conn_must_fail`; function body calls `a.Conn()`. Building under that tag MUST fail (FR-103b, SC-104, contract § Compile-time guarantees item 3).
- [X] T017 [US4] Update `integration/compiletest/README.md` to document the two new tags (`awareness_requestmany_must_fail`, `awareness_conn_must_fail`) alongside the existing `awareness_publish_must_fail`. Include the `go vet -tags=...` recipe.
- [X] T018 [US4] Add a CI helper script or extend the existing test target (whichever `001` uses — check `Makefile`) so `make test` (or a dedicated `make compile-deny`) runs `go vet -tags=<tag> ./integration/compiletest/...` for each of the three tags and asserts non-zero exit. Wire it so a regression that *adds* the forbidden method on `AwarenessContext` flips the build to red. (R-109.)
- [X] T019 [US4] Integration test `TestReasoning_HasFullSurface` in `integration/request_many_test.go` (creating the file) covering US-4 AS-3: an imp whose reasoning successfully invokes `r.RequestMany(...)` against one responder and `r.Publish(...)` against an unrelated subject — confirms the methods exist and work on `ReasoningContext`.

### Implementation for User Story 4

No production code beyond Phase 2 — this story is enforcement-by-absence plus a small CI gate.

**Checkpoint**: All three forbidden methods on awareness fail to compile under their respective tags. Reasoning's full surface is reachable.

---

## Phase 6: User Story 2 — Reasoning fans out via RequestMany (Priority: P1)

**Goal**: `r.RequestMany(ctx, subject, payload, opts...)` publishes once, collects replies within the effective window (or up to `WithRequestManyMax(k)`), and returns the slice. Empty slice is the legitimate "nobody home" return.

**Independent Test**: Embedded NATS, three plain `nc.Subscribe("health.ping", ...)` responders each replying with their own ID, reasoning calls `r.RequestMany(ctx, "health.ping", nil, WithRequestManyWindow(200*time.Millisecond))` — assert exactly three replies collected, call returned within `200 ms + ε`, no error (spec.md US-2 Independent Test, SC-102).

### Tests for User Story 2

- [X] T020 [US2] Integration test `TestRequestMany_HappyPath` in `integration/request_many_test.go` (append to file from T019) covering US-2 AS-1: register three responders on `health.ping`, call `r.RequestMany(..., WithRequestManyWindow(200*time.Millisecond))`, assert (a) `len(replies) == 3`, (b) replies contain each responder's distinct payload (order-insensitive — the harness does not sort), (c) elapsed time ∈ `[200ms, 200ms + small ε]`, (d) `Metrics.RequestManyCalls` incremented by one.
- [X] T021 [US2] Integration test `TestRequestMany_MaxCapEarlyExit` in the same file covering US-2 AS-2 and FR-111: register five responders on a subject, call `RequestMany(..., WithRequestManyWindow(500*time.Millisecond), WithRequestManyMax(3))`, assert `len(replies) == 3` and elapsed time is much less than 500 ms (the cap fires well before the window).
- [X] T022 [US2] Integration test `TestRequestMany_WindowElapseNoResponders` in the same file covering US-2 AS-3: no responders subscribed, call `RequestMany(..., WithRequestManyWindow(100*time.Millisecond))`, assert `len(replies) == 0`, `err == nil`, elapsed time ∈ `[100ms, 100ms + small ε]`. This confirms an empty slice is the legitimate fan-out "all-quiet" outcome (FR-110).
- [X] T023 [US2] Integration test `TestRequestMany_InboxCleanup` in the same file covering FR-113: across the three preceding subtests, assert via `nc.NumSubscriptions()` (or equivalent) that the connection has no leaked subscriptions after each call returns. Inbox MUST be unsubscribed on every return path.

### Implementation for User Story 2

Implementation already landed in Phase 2 (T008, T010). T020–T023 only verify behavior.

**Checkpoint**: Fan-out works. Window-elapse, k-cap early exit, and no-responder cases all behave per spec, with no inbox leak.

---

## Phase 7: User Story 5 — Error categories are NATS-native and distinct (Priority: P2)

**Goal**: `*ErrNoResponders` and `*ErrRequestTimeout` are produced from controlled scenarios and distinguishable via `errors.As`. `*ErrRequestTimeout` unwraps to `context.DeadlineExceeded` (FR-107, FR-118).

**Independent Test**: Two controlled scenarios — `Request` against a subject with no subscribers (expect `*ErrNoResponders`) and `Request` against a delayed responder under a short per-call timeout (expect `*ErrRequestTimeout`). `errors.As` distinguishes them (spec.md US-5 Independent Test, SC-105).

### Tests for User Story 5

- [X] T024 [US5] Integration test `TestRequest_ErrNoResponders` in `integration/request_errors_test.go` (creating the file) covering US-5 AS-1: no subscriber registered for the subject, reasoning calls `r.Request(ctx, "nobody.home", nil)`, assert (a) `errors.As(err, &noResp)` succeeds, (b) `noResp.Subject == "nobody.home"`, (c) elapsed time is well under the default request timeout (substrate signals immediately), (d) `Metrics.RequestNoResponders == 1`.
- [X] T025 [US5] Integration test `TestRequest_ErrRequestTimeout` in the same file covering US-5 AS-2: register a slow responder (200 ms delay), call `r.Request(ctx, "slow", nil, WithRequestTimeout(50*time.Millisecond))`, assert (a) `errors.As(err, &toErr)` succeeds, (b) `toErr.Subject == "slow"`, (c) `toErr.Timeout == 50ms`, (d) `errors.Is(err, context.DeadlineExceeded)` succeeds, (e) elapsed ∈ `[50ms, 50ms + small ε]`, (f) `Metrics.RequestTimeouts == 1`.
- [X] T026 [US5] Integration test `TestRequest_CtxCanceled` in the same file covering US-5 AS-3: register a slow responder, the caller's `ctx` is cancelled while the request is in flight (cancel after ~20 ms against a 200 ms delay), assert `errors.Is(err, context.Canceled)` succeeds and the error is NOT `*ErrRequestTimeout` (cancellation is standard Go semantics, not a framework category).

### Implementation for User Story 5

No new production code — error translation already implemented in Phase 2 (T008). Tests are pure verification.

**Checkpoint**: Both error categories are reachable, distinguishable, and their counters move correctly.

---

## Phase 8: User Story 6 — Per-call timeout override; no harness retry (Priority: P2)

**Goal**: A per-call `WithRequestTimeout(T)` overrides the harness default for that call. The harness performs no retry on failure — a timeout failure does NOT produce a second NATS publish on the same subject (FR-109, SC-106).

**Independent Test**: Subscribe a 200 ms delayed responder; call `Request` once with the default timeout (long; succeeds) and once with `WithRequestTimeout(50*time.Millisecond)` (times out). Assert the second call returns `*ErrRequestTimeout` and that the harness did not publish a retry (count messages received by the subscriber) (spec.md US-6 Independent Test).

### Tests for User Story 6

- [X] T027 [US6] Integration test `TestRequest_PerCallTimeoutPrecedence` in `integration/request_timeout_test.go` (creating the file) covering US-6 AS-1 and AS-2: with `WithDefaultRequestTimeout(1*time.Second)` and a responder replying in ~10 ms, (a) `r.Request(ctx, subj, payload)` succeeds (uses default), (b) `r.Request(ctx, subj, payload, WithRequestTimeout(5*time.Millisecond))` against a 200 ms-delayed responder returns `*ErrRequestTimeout` whose `Timeout == 5ms` regardless of the (much longer) default.
- [X] T028 [US6] Integration test `TestRequest_NoRetryOnTimeout` in the same file covering US-6 AS-3 and SC-106: subscribe a counting responder on `count.me` that records every received request; call `r.Request(ctx, "count.me", nil, WithRequestTimeout(50*time.Millisecond))` once against a 200 ms-delayed handler so the call times out, assert the responder received **exactly one** request (the harness did not retry).
- [X] T029 [US6] Integration test `TestRequestMany_PerCallWindowAndMax` in the same file covering US-6 AS-4 and AS-5: confirm `WithRequestManyWindow(T)` overrides `WithDefaultRequestManyWindow` for one call, and `WithRequestManyMax(k)` returns immediately on the k-th reply (overlaps with T021 but binds the override semantics to US-6's "policy-free" framing).

### Implementation for User Story 6

No new production code — precedence and "no retry" already implemented in Phase 2 (T008). Tests are verification.

**Checkpoint**: Per-call overrides win over harness defaults; the harness is policy-free.

---

## Phase 9: User Story 7 — Subjects are literal (Priority: P2)

**Goal**: `Request`, `RequestMany`, and `Publish` all send on the declared subject verbatim. No framework prefix, no platform-mode segment, no transformation (FR-115, constitution v2.2.0).

**Independent Test**: Run an imp with no subject-prefix option (there is no such option after the v2.2.0 cleanup). Subscribe a substrate-side listener on the literal subject `knowledge.recall`. Call `r.Request(ctx, "knowledge.recall", payload)` from reasoning. Assert the substrate-side listener received the request on `knowledge.recall` exactly (spec.md US-7 Independent Test).

### Tests for User Story 7

- [X] T030 [US7] Integration test `TestSubjectsAreLiteral_Request` in `integration/request_test.go` (append to the file from T012/T013) covering US-7 AS-1 for `Request`: subscribe a capturing handler on the literal subject `knowledge.recall` (record `msg.Subject`); reasoning calls `r.Request(ctx, "knowledge.recall", payload)`; assert the captured `msg.Subject == "knowledge.recall"` exactly, and the `*ErrNoResponders.Subject` (in a negative variant where no responder exists) is also the verbatim subject. Repeat the same assertion for the awareness `a.Request` path.
- [X] T031 [US7] Integration test `TestSubjectsAreLiteral_RequestMany_Publish` in `integration/request_many_test.go` (append) extending US-7 AS-1 to `RequestMany` and `Publish`: assert each method's wire subject matches the declared subject byte-for-byte. Capture `msg.Subject` on the responder side.

> US-7 AS-2 (cross-account import) is operator-substrate behavior; the framework's part of the contract — "no transformation" — is exercised by T030/T031. No code change beyond the in-tree assertions is needed.

### Implementation for User Story 7

No new production code — verbatim-subject behavior already follows from Phase 2 (T008). Tests prove it.

**Checkpoint**: Subjects pass through verbatim across all three outbound primitives.

---

## Phase 10: SC-107 — No-Request imps are unchanged

**Goal**: An imp that calls neither `Request` nor `RequestMany` nor `Publish` produces the same observable substrate footprint and metrics movement as in `001-harness-core` (SC-107).

**Independent Test**: Construct a pure-awareness imp doing only state updates, drive a message through, assert that the new counters (`RequestCalls`, `RequestManyCalls`, `RequestNoResponders`, `RequestTimeouts`) all remain at zero, and that the existing 001 counters move identically to a 001-baseline imp.

### Tests for SC-107

- [X] T032 [P] Integration test `TestNoRequest_FootprintUnchanged` in `integration/request_nodeps_test.go` (creating the file): construct an imp whose awareness returns a verdict without calling `Request`, and whose reasoning either is absent or makes no outbound calls; drive several messages through; assert the four new metrics counters all equal zero, and the existing 001 counters (dispatch, drops, etc.) move per the pre-existing observability contract.

**Checkpoint**: A 001-shaped imp on the 002 binary behaves identically.

---

## Phase 11: Polish & Cross-Cutting Concerns

**Purpose**: Final tidy-up across the feature.

- [X] T033 [P] Re-read `harness/doc.go` after all phases land and confirm the package godoc still covers the surface accurately (`Request`, `RequestMany`, `Publish`, `Conn()`, two typed errors, two construction options).
- [X] T034 [P] If any DEBUG/WARN log lines from the dispatch helpers (T008) differ from the contract's log spec (`"request"`, `"request_many"`, `"request failed"` with documented fields), bring them in line. Verify a sample via the existing `observability_test.go` pattern in `harness/`.
- [ ] T035 Run the quickstart end-to-end manually: copy the awareness + reasoning + main from `specs/002-capability-client/quickstart.md` into a scratch program, run against a local `nats-server`, exercise `Request`, `RequestMany`, and `Publish`. Confirm every error path described in "Driving each error category deterministically" reproduces. Do NOT commit the scratch program. _(Deferred: every error category and outbound primitive is already covered end-to-end by the embedded-nats integration suite — TestRequest_ErrNoResponders, TestRequest_ErrRequestTimeout, TestRequest_CtxCanceled, TestRequestMany_{HappyPath,WindowElapseNoResponders,MaxCapEarlyExit}, TestReasoning_HasFullSurface. Run the manual quickstart before tagging a release if external-substrate parity becomes a concern.)_
- [X] T036 Final `make fmt && make test && make lint` from repo root, then run the three compile-deny vet commands explicitly (`go vet -tags=awareness_publish_must_fail`, `…_requestmany_must_fail`, `…_conn_must_fail` against `./integration/compiletest/...`) and confirm each exits non-zero.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; can start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion. BLOCKS all user stories — T002–T011 must all land before any US-phase task compiles.
- **User Stories (Phases 3–10)**: All depend on Foundational completion. Per `plan.md`, the recommended landing order is US-1 → US-3 → US-4 → US-2 → US-5 → US-6 → US-7 → SC-107. With sufficient parallelism, US-5, US-6, US-7, and SC-107 can be worked in parallel after US-1/US-2/US-3/US-4 land, because each only adds test files that depend on the shared Phase-2 implementation.
- **Polish (Phase 11)**: Depends on all user-story phases.

### User Story Dependencies

- **US-1 (P1) — Reasoning Request**: No story dependencies; first MVP slice.
- **US-3 (P1) — Awareness Request**: Shares Phase-2 helpers with US-1; can land before or after US-1's test, but `plan.md` recommends after US-1's test for incremental validation.
- **US-4 (P1) — Compile-time absence**: Independent of US-1/US-2/US-3 implementation; needs only that `AwarenessContext` interface (T009) is final. CI gate (T018) depends on T015/T016 existing.
- **US-2 (P1) — Reasoning RequestMany**: Independent of US-1/US-3 logically but shares the dispatch file (T008). Land tests after US-1/US-3 tests so failures isolate cleanly.
- **US-5 (P2) — Error categories**: Depends on US-1 happy-path landing first (so failure paths can be contrasted with success).
- **US-6 (P2) — Per-call timeout / no retry**: Depends on US-5 (timeout is a category before it is an override target).
- **US-7 (P2) — Subjects are literal**: Independent; could land any time after Phase 2.
- **SC-107**: Independent; could land any time after Phase 2.

### Within Each User Story

- The Phase-2 foundation already contains the production code; user-story phases are predominantly test additions.
- Tests within a story can be written and run in any internal order, but they MUST verify behavior that is already implemented (TDD-style "fail-first" applies to the few foundation-level pieces that are still being built; once T008–T011 land, tests are written to pass).
- Add new test files first (T012, T015/T016, T019, T024, T027, T030, T032), then append additional subtests to those files in subsequent tasks.

### Parallel Opportunities

- **Phase 2 fan-out**: T002, T003, T004, T007, T011 can be authored in parallel (different files). T005 → T006 sequential (option declaration before validation). T008 depends on T002 and T007. T009/T010 depend on T008. T011 is godoc — can land last but in parallel with tests.
- **Phase 5 (US-4)**: T015 and T016 are independent files and can land in parallel; T017 (README) follows; T018 (CI gate) follows T015/T016; T019 is independent of the compile-deny files.
- **Phases 7–10 in parallel**: US-5 (T024–T026), US-6 (T027–T029), US-7 (T030–T031), and SC-107 (T032) touch four different test files and can be developed in parallel once Phase-2 helpers are stable.
- **Phase 11**: T033 and T034 in parallel; T035 manual; T036 final.

---

## Parallel Example: Phase 2 Foundational

```bash
# Independent files — author concurrently:
Task: "T002 Add ErrNoResponders / ErrRequestTimeout to harness/errors.go"
Task: "T003 Add four counter fields to harness/metrics_internal.go"
Task: "T004 Extend Metrics snapshot struct in harness/spec.go"
Task: "T007 Define RequestOption / RequestManyOption in new harness/request.go (option types only)"
Task: "T011 Update harness/doc.go package godoc"
```

```bash
# After T002, T003, T007 land — implement helpers:
Task: "T008 Implement requestSingle and requestMany in harness/request.go"
```

```bash
# After T008 — wire into contexts:
Task: "T009 Add Request to awarenessCtx in harness/context_awareness.go"
Task: "T010 Add Request and RequestMany to reasoningCtx in harness/context_reasoning.go"
```

## Parallel Example: User Story 4 (Compile-time guarantees)

```bash
# Independent files — author concurrently:
Task: "T015 Create integration/compiletest/awareness_no_requestmany.go"
Task: "T016 Create integration/compiletest/awareness_no_conn.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001).
2. Complete Phase 2: Foundational (T002–T011) — the critical block.
3. Complete Phase 3: User Story 1 (T012).
4. **STOP and VALIDATE**: `make fmt && make test && make lint`. Reasoning `Request` works end-to-end.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US-1 (T012) → reasoning `Request` happy path → demo / validate.
3. US-3 (T013–T014) → awareness `Request` → validate.
4. US-4 (T015–T019) → compile-time guarantees + reasoning surface smoke → validate.
5. US-2 (T020–T023) → fan-out → validate.
6. US-5 (T024–T026) → error categories → validate.
7. US-6 (T027–T029) → per-call overrides + no-retry → validate.
8. US-7 (T030–T031) → literal-subject assertions → validate.
9. SC-107 (T032) → 001-baseline parity → validate.
10. Polish (T033–T036) → final clean-up.

### Parallel Team Strategy

With two or three contributors after Phase 2 lands:

- Contributor A: US-1 (T012) → US-3 (T013–T014) → US-7 (T030–T031).
- Contributor B: US-4 (T015–T019) → SC-107 (T032).
- Contributor C: US-2 (T020–T023) → US-5 (T024–T026) → US-6 (T027–T029).

Test files are partitioned so contributors do not collide on the same file.

---

## Notes

- [P] tasks operate on different files or are otherwise independent.
- The "implementation" sections of US-1, US-3, US-2, US-5, US-6, US-7, and SC-107 are intentionally empty: the feature's design pushes all production code into the Foundational phase (T008–T011). User-story phases are predominantly verification, which mirrors the spec's "Independent Test" structure.
- Test artifacts that span multiple stories share a file: `integration/request_test.go` holds US-1, US-3, and US-7 (the `Request` path); `integration/request_many_test.go` holds US-2, US-4 (reasoning smoke), and US-7 fan-out subjects. This keeps the test surface readable.
- Compile-time absence guarantees are real assertions, not informational comments — the CI gate at T018 makes a regression visible.
- `RequestMany` inbox cleanup (FR-113) MUST be exercised on every return path; T023 is non-negotiable.
- Do not introduce retry, backoff, or codec helpers — FR-NS-103 / FR-NS-104 are explicit non-requirements.
- Commit after each task (the `after_tasks` hook is `git commit`; commit cadence inside the feature is the contributor's call).
