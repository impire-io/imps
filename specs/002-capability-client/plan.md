# Implementation Plan: Request/Reply Surface

**Branch**: `002-capability-client` | **Date**: 2026-05-11 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-capability-client/spec.md`

> **Branch name note**: the branch still reads `002-capability-client`
> because this work began as a capability-client design. The redesign
> dropped the capability-declaration model in favor of direct NATS
> primitives (`Request`, `RequestMany`, `Publish`). The branch name is
> kept for git continuity; the feature is the request/reply surface.

## Summary

This feature delivers the outbound NATS surface for imps running on the
`001-harness-core` substrate. Three primitives are added across the two
context types: **awareness gets `Request`** (single round trip — bounded
by call shape), **reasoning gets `Request`, `RequestMany`, and `Publish`**
(`Publish` was already there from 001, unchanged). The energy gradient
is enforced structurally: `RequestMany`, `Publish`, and `Conn` do not
exist on `AwarenessContext`, so awareness code attempting to fan out,
fire-and-forget, or grab the raw connection fails to compile. Three
build-tagged files under `integration/compiletest/` assert this — one
per forbidden method (`awareness_no_publish.go` inherited from 001 plus
`awareness_no_requestmany.go` and `awareness_no_conn.go` added here).

The feature deliberately holds no capability concept. There is no
`Capabilities` section on `ImpSpec`, no `$SRV.INFO` discovery, no
boundedness metadata, no resolved surface, no in-tree capability fixture.
The imp names subjects directly; what's on the other end is the
operator's design, not the framework's concern. This is a direct
consequence of constitution v2.2.0's two principles — "The energy
gradient is structural" (call-shape boundedness, not metadata) and
"Imps see one subject path" (no resolver, literal subjects; cross-account routing
is the substrate's job).

Subjects pass through verbatim — the harness performs no
transformation, consistent with constitution v2.2.0 and the 001
cleanup that removed `WithSubjectPrefix`, the internal resolver, and
`WithPlatformMode`. Errors are two NATS-native typed sentinels:
`*ErrNoResponders` and `*ErrRequestTimeout`. No retry, no backoff, no
circuit-breaker (FR-NS-103). The framework's surface is bytes in, bytes
out — symmetric with `Publish` — and imp authors who want typed
payloads write their own marshal/unmarshal (FR-NS-104).

The full set of design decisions is in [`research.md`](./research.md);
the runtime types are in [`data-model.md`](./data-model.md); the binding
contract is at [`contracts/request-reply.md`](./contracts/request-reply.md);
and the imp-author walkthrough is in [`quickstart.md`](./quickstart.md).

## Technical Context

**Language/Version**: Go 1.25, inherited from `001-harness-core` (R-1).

**Primary Dependencies**:

- `github.com/nats-io/nats.go` — already pinned (R-2 of 001). The new
  code uses `nc.RequestWithContext` for single-shot requests and
  `nc.NewRespInbox` + `nc.ChanSubscribe` + `nc.PublishMsg` for the
  collect-with-window pattern in `RequestMany`.
- `github.com/nats-io/nats-server/v2` — already pinned for embedded
  tests.
- `log/slog` — inherited.
- No new third-party dependencies. (`github.com/nats-io/nats.go/micro`
  is NOT imported — the earlier draft used it for `Info`/`EndpointInfo`
  during $SRV.INFO aggregation; that's gone.)

**Storage**: None. The feature is purely synchronous request/reply
plumbing. No persistent state, no per-imp caches, no resolved surface.

**Testing**: Go standard `testing` with table-driven subtests. Tests
register responders directly via `nc.Subscribe(...)` (no dedicated
fixture package needed; the earlier `testutil/capfixture` is dropped).
The compile-time invariants live in `integration/compiletest/` —
`awareness_no_publish.go` (existing) and `awareness_no_requestmany.go`
(new). Run discipline: `make fmt && make test && make lint`.

**Target Platform**: Linux and macOS, single-process Go runtime,
in-process with the imp's awareness and reasoning code. Inherited from
`001-harness-core`.

**Project Type**: Go library, single module, single public package
(`harness`) plus the existing internal sub-packages. No new public or
internal sub-packages introduced by this feature (R-110). The
developer-facing import remains `github.com/impire-io/imps/harness`.

**Performance Goals**:

- `Request` round-trip under 100 ms on local embedded NATS (SC-101 /
  SC-103).
- `RequestMany` with three local responders and a 200 ms window
  returns within `200 ms + small ε` (SC-102).
- `Request` with a 50 ms per-call timeout against a 200 ms-delayed
  responder returns `*ErrRequestTimeout` within `50 ms + small ε`
  (SC-106).

**Constraints**:

- No retry, no backoff, no circuit-breaker (FR-NS-103).
- No JSON/codec layer at the framework level (FR-NS-104).
- No service-error-header decoding; application errors are the
  responder's protocol (FR-NS-105).
- No streaming (FR-NS-106). `RequestMany` is collect-with-window only.
- No queue-group policy in the client (FR-NS-107).
- No per-call headers in v1 (FR-NS-108).
- No capability declaration, no discovery, no surface partitioning,
  no boundedness metadata. The imp talks to subjects.

**Scale/Scope**: An imp issues `Request`s during awareness (a few per
dispatch, bounded by call shape) or reasoning (any number, no
framework-imposed bound — same as `Publish` today). `RequestMany`
collects up to whatever the operator's deployment produces, capped at
the per-call `WithRequestManyMax(k)` or the window.

## Constitution Check

*Gate evaluated against the Load-Bearing Commitments, Working Principles, and Non-Negotiables in `.specify/memory/constitution.md` v2.2.0.*

### Load-Bearing Commitments

| Commitment | Pass? | Evidence |
|---|---|---|
| Imps stay small and agile | ✅ | This feature is the smallest possible outbound surface (three methods, two typed errors, two construction options). No new internal sub-package (R-110). No discovery, no resolved surface, no fixture — all the ceremony from the earlier draft is gone. The developer-facing import remains one package. |
| The energy gradient is structural | ✅ | `AwarenessContext` exposes `Request` only. `ReasoningContext` exposes `Request`, `RequestMany`, `Publish`, and `Conn`. Three build-tagged compile tests (`awareness_no_publish.go`, `awareness_no_requestmany.go`, `awareness_no_conn.go`) assert at build time that the forbidden methods do not exist on `AwarenessContext`. The boundedness criterion is the **call shape**, not endpoint metadata (R-101) — a single request/reply is bounded by virtue of being a single request/reply. |
| Capabilities are external; the harness is small | ✅ | No capability implementation lives in this feature. Crucially, no capability *abstraction* lives in this feature either — the harness does not model "capabilities" at all. Whatever is on the other end of a subject is the operator's concern. The harness adds three methods; the framework gets smaller, not larger. |
| Coordination happens through the soulstream | N/A here | This feature does not introduce inter-imp coordination. Soulstream remains deferred. |
| Wire protocols are per-capability; deployment shape is uniform | ✅ | The byte-level surface (`Request`/`RequestMany`/`Publish`) imposes no wire-protocol uniformity. Each responder defines its own request/reply shape; the framework consumes bytes (R-112). Deployment shape is uniform per constitution v2.2.0 "Imps see one subject path": the imp's declared subject is the wire subject — the framework imposes no transformation. |

### Working Principles

The refined "Imps see one subject path" principle (v2.2.0) is the
design's load-bearing assumption. `Request`/`RequestMany`/`Publish` all
send on the declared subject verbatim — no framework transformation.
Operators configure NATS account imports to make cross-account
responders reachable on the imp's local subjects.

### Non-Negotiables

| Rule | Pass? | Evidence |
|---|---|---|
| Awareness does not call unbounded capabilities | ✅ | The structural mechanism: `AwarenessContext` has `Request` only. `RequestMany` (fan-out, unbounded by responder count + window) is absent. `Publish` (fire-and-forget, side-effecting) is absent (inherited from 001). `Conn` (raw-`*nats.Conn` escape hatch) is absent — awareness cannot bypass the bounded surface by reaching for the underlying connection. Build-tagged compile tests (one file per forbidden method) assert all three absences. |
| Imps do not share local memory | ✅ | Nothing in this feature lets two imps share state. |
| Capability services do not persist per-request data | N/A here | Service-side concern; not bound by this feature. |
| Direct provider/SDK calls in imp code are forbidden | ✅ | All outbound work goes via NATS subjects through the harness's three methods. No LLM SDK, no DB driver introduced. |
| No central registry beyond NATS micro | ✅ | This feature introduces *no* discovery mechanism at all. The imp names subjects directly. (`$SRV.INFO` is NATS micro's discovery surface; this feature does not use it. Future capability-aware imps that want to enumerate available services may use it, but the framework does not consume it on their behalf.) |
| No generic capability protocol | ✅ | The byte-level surface imposes no protocol. Capabilities (and any other responder) design their own request/reply schemas; the framework consumes bytes. NATS micro's `Nats-Service-Error` headers are NOT decoded by the framework — that is a per-capability protocol concern. |
| Stubs and partial implementations are never reported as complete | ✅ commitment | Implementation discipline. The plan does not include stubs as deliverables. Compile-time checks are real assertions. `RequestMany` inbox cleanup is exhaustive (every return path unsubscribes — FR-113), not "we'll add cleanup later". |

**Gate result**: **PASS** — both pre-research and post-design (re-checked
after producing `data-model.md` and `contracts/`). No justified
violations; the Complexity Tracking table below is empty.

## Project Structure

### Documentation (this feature)

```text
specs/002-capability-client/
├── plan.md                          # This file (/speckit-plan output)
├── research.md                      # Phase 0 — 12 design decisions with rationale
├── data-model.md                    # Phase 1 — interface extensions, options, errors, metrics
├── quickstart.md                    # Phase 1 — imp-author walkthrough
├── contracts/
│   └── request-reply.md             # Single contract: public API, dispatch, errors, observability
├── spec.md                          # Feature spec (rewritten under the new design)
└── tasks.md                         # Phase 2 output (/speckit-tasks command — NOT created by this command)
```

> The earlier draft of this directory included
> `contracts/capability-client.md`, `contracts/discovery.md`, and
> `contracts/fixture.md`. All three are deleted; their content is either
> dropped (discovery, fixture) or folded into `contracts/request-reply.md`
> (public API).

### Source Code (repository root)

Files added or modified for this feature, against the `001-harness-core`
baseline (after the v2.2.0 cleanup that removed `WithPlatformMode`, `WithSubjectPrefix`, the resolver, and the `Actions` whitelist, and added `ReasoningContext.Conn()`).

```text
go.mod                               # unchanged
go.sum                               # unchanged (no new deps)

harness/                             # public package — developer-facing API
├── context.go                       # MODIFIED — extend AwarenessContext with Request;
│                                     #            extend ReasoningContext with Request, RequestMany
├── context_awareness.go             # MODIFIED — concrete Request method;
│                                     #            delegates to harness/request.go helpers
├── context_reasoning.go             # MODIFIED — concrete Request + RequestMany methods;
│                                     #            both delegate to harness/request.go helpers
├── request.go                       # NEW — shared dispatch helpers:
│                                     #        - effective-timeout / effective-window resolution
│                                     #        - single-shot request via nc.RequestWithContext
│                                     #        - fan-out collect-with-window via inbox subscription
│                                     #        - error translation (no-responders, timeout)
│                                     #        - RequestOption / RequestManyOption types and constructors
├── errors.go                        # MODIFIED — add ErrNoResponders, ErrRequestTimeout
├── options.go                       # MODIFIED — add WithDefaultRequestTimeout, WithDefaultRequestManyWindow;
│                                     #            extend runtimeOptions; defaults applied in
│                                     #            defaultRuntimeOptions(); validation in bootRuntime
├── imp.go                           # MODIFIED — bootRuntime validates the two new options;
│                                     #            metrics snapshot extended
├── metrics_internal.go              # MODIFIED — add RequestCalls, RequestManyCalls,
│                                     #            RequestNoResponders, RequestTimeouts counters
├── spec.go                          # MODIFIED — Metrics struct (public snapshot) extended
└── doc.go                           # MODIFIED — package godoc covers the outbound surface

internal/                            # NO new sub-packages
└── (existing sub-packages unchanged)

integration/                         # black-box tests
├── request_test.go                  # NEW — US-1 (reasoning Request happy path),
│                                     #        US-3 (awareness Request happy path),
│                                     #        US-7 (subject resolution)
├── request_many_test.go             # NEW — US-2 (reasoning RequestMany happy path, k-cap, window-elapse)
├── request_errors_test.go           # NEW — US-5 (ErrNoResponders, ErrRequestTimeout categories)
├── request_timeout_test.go          # NEW — US-6 (per-call timeout, no retry on timeout)
├── request_nodeps_test.go           # NEW — SC-107 (imp that issues no Request runs identically to 001)
└── compiletest/
    ├── awareness_no_publish.go      # UNCHANGED (existing assertion from 001)
    ├── awareness_no_requestmany.go  # NEW — build-tagged file; building under
    │                                 #  tag awareness_requestmany_must_fail MUST fail
    │                                 #  (compile-time absence of RequestMany on AwarenessContext — SC-104)
    ├── awareness_no_conn.go         # NEW — build-tagged file; building under
    │                                 #  tag awareness_conn_must_fail MUST fail
    │                                 #  (compile-time absence of Conn on AwarenessContext — SC-104, FR-103b)
    └── README.md                    # MODIFIED — documents all three tags + go vet recipe

testutil/                            # UNCHANGED
└── natstest/                        # the existing embedded-NATS helper covers all test needs
                                     # (no testutil/capfixture introduced)

examples/                            # unchanged in this feature; a follow-up example imp
                                     # exercising Request/RequestMany may land alongside an
                                     # inference or knowledge feature
docs/                                # unchanged here; docs/02-capability-service-pattern.md
                                     # was annotated with the import-rewrites-subjects note
                                     # in the same constitution-v2.2.0 cleanup
```

**Structure Decision**: The implementation is small enough to live
entirely in `harness/` without a new internal sub-package (R-110). One
new file — `harness/request.go` — holds the dispatch helpers shared
between awareness's `Request` and reasoning's `Request` / `RequestMany`.
No fixture package (R-108); tests register raw `nc.Subscribe` handlers.
The compile-time test surface grows by one file under
`integration/compiletest/` mirroring the established 001 pattern.

## Phase 2: task strategy (preview only — `/speckit-tasks` is a separate command)

`/speckit-plan` stops at Phase 1. The task plan below is descriptive —
to be expanded by `/speckit-tasks` into the per-task `tasks.md`. The
expected task strategy follows the user-story dependency order:

1. **US-1 (P1) — Reasoning Request.** Types: `RequestOption`,
   `requestOptions`, `*ErrNoResponders`, `*ErrRequestTimeout`. Options:
   `WithRequestTimeout`, `WithDefaultRequestTimeout`. Helpers in
   `harness/request.go`: effective-timeout, dispatch via
   `nc.RequestWithContext`, error translation. Interface +
   concrete-type changes for reasoning. Integration test for
   happy-path Request.

2. **US-3 (P1) — Awareness Request.** Same helpers, reused from US-1.
   Interface + concrete-type changes for awareness. Integration test
   for happy-path Request from awareness.

3. **US-4 (P1) — Compile-time absence of RequestMany, Publish, and Conn on awareness.**
   Two new build-tagged files:
   `integration/compiletest/awareness_no_requestmany.go` (tag
   `awareness_requestmany_must_fail`) and
   `integration/compiletest/awareness_no_conn.go` (tag
   `awareness_conn_must_fail`). The third file
   (`awareness_no_publish.go`, tag `awareness_publish_must_fail`) is
   inherited from `001-harness-core` and is re-asserted here without
   modification. CI gate extended to assert all three tagged builds
   fail (mirrors the existing `awareness_publish_must_fail` pattern).

4. **US-2 (P1) — Reasoning RequestMany.** Types:
   `RequestManyOption`, `requestManyOptions`. Options:
   `WithRequestManyWindow`, `WithRequestManyMax`,
   `WithDefaultRequestManyWindow`. Helper in `harness/request.go`:
   collect-with-window loop using
   `nc.NewRespInbox` + `nc.ChanSubscribe` + `nc.PublishMsg`. Inbox
   cleanup on every return path (FR-113). Integration tests for
   happy-path RequestMany, k-cap early exit, window-elapse with
   zero replies.

5. **US-5 (P2) — Error categories.** Translation logic for
   `nats.ErrNoResponders` → `*ErrNoResponders`,
   `context.DeadlineExceeded` (timeout branch) → `*ErrRequestTimeout`.
   `errors.Is`/`errors.As` round-trip tests. Integration test
   `request_errors_test.go`.

6. **US-6 (P2) — Per-call timeout, no retry.** Per-call option
   precedence over the harness default; assert that a timeout failure
   does NOT produce a second NATS request (subscribe on the resolved
   subject in the test and count). Integration test
   `request_timeout_test.go`.

7. **US-7 (P2) — Subjects are literal.** Tests assert all three methods
   send on the declared subject verbatim. Captured by subscribing the
   test on the literal form (no prefix) and verifying delivery.

8. **SC-107 — No-Request imps unchanged.** Integration test
   `request_nodeps_test.go` confirms an imp that never calls
   `Request`/`RequestMany`/`Publish` produces the same observable
   substrate footprint and metrics movement as in `001-harness-core`.

Cross-cutting:

- `harness/metrics_internal.go` + `harness/spec.go` Metrics extensions:
  four counters (`RequestCalls`, `RequestManyCalls`,
  `RequestNoResponders`, `RequestTimeouts`).
- `harness/options.go` validation in `bootRuntime` for the two new
  defaults (non-positive → `*ErrConfigInvalid`).
- `harness/doc.go` package godoc extension.

The `/speckit-tasks` command derives the actual `tasks.md` from this
strategy plus `data-model.md` and the single contract file; the present
plan only constrains the high-level shape.

## Complexity Tracking

> No constitutional violations require justification. Table intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
