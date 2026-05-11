# Feature Specification: Request/Reply Surface

**Feature Branch**: `002-capability-client`
**Created**: 2026-05-11
**Status**: Draft
**Input**: User description: "How an imp talks to anything outside itself over NATS — `Request`, `RequestMany`, `Publish`. No capability-declaration ceremony; rely on what NATS already provides. The energy gradient is enforced by which call shapes each context exposes."

## Overview

This feature delivers the **outbound NATS surface** for imps running on the
`001-harness-core` substrate: how an imp's awareness and reasoning code
issues request/reply and publish operations against arbitrary NATS
subjects, with the energy gradient enforced by the call shape that each
context type makes available.

The ground rules:

- **Awareness sees one outbound primitive: `Request`.** A single
  request/reply round trip is bounded by its very shape — one publish,
  one reply, one effective deadline. Awareness invokes services this way
  inside the dispatch hot path.
- **Reasoning sees the full outbound surface: `Request`, `RequestMany`,
  `Publish`.** `RequestMany` is fan-out (collect replies from many
  responders within a window) and `Publish` is fire-and-forget side
  effecting; both are reasoning-only.
- **No capability declaration. No discovery. No `$SRV.INFO`.** Imps name
  subjects directly. If nobody handles a subject, the call returns
  `*ErrNoResponders`. If a service does handle it, the imp gets the
  reply. The framework does not model "what's on the other end" —
  responders are NATS services; their pattern (capability service,
  bridge, peer imp, ad-hoc tool) is the operator's design, not the
  framework's concern.
- **Subjects are literal** (constitution v2.2.0 "Imps see one subject
  path"). `Request`/`RequestMany`/`Publish` all send on the declared
  subject verbatim — the framework imposes no prefix, no platform-mode
  segment, no transformation. Cross-account routing is configured at the
  substrate via NATS account imports.

The structural enforcement of the energy gradient becomes:

| Method | AwarenessContext | ReasoningContext |
|---|---|---|
| `Request(ctx, subject, payload, opts...)` | ✅ | ✅ |
| `RequestMany(ctx, subject, payload, opts...)` | ❌ (not on the interface) | ✅ |
| `Publish(ctx, subject, payload)` | ❌ (not on the interface — inherited from 001) | ✅ (no framework whitelist; substrate ACLs gate) |
| `Conn() *nats.Conn` | ❌ (not on the interface) | ✅ (escape hatch for generic NATS clients, added in the 001 v2.2.0 cleanup) |

A developer who writes `awareness.Publish(...)`,
`awareness.RequestMany(...)`, or `awareness.Conn()` gets a compile error
because those methods do not exist on `AwarenessContext`. The same
compile-time discipline `001-harness-core` established for `Publish`
extends to `RequestMany` and `Conn`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reasoning reaches a service via Request (Priority: P1)

A developer's reasoning function invokes `r.Request(ctx, "knowledge.recall", payload)`. The harness issues a single NATS request on the literal subject `knowledge.recall` and returns the reply payload bytes. The call is one-shot — no retry, no backoff, no circuit-breaker.

**Why this priority**: This is the basic promise of the feature. Every other behavior (fan-out, error categories, awareness's bounded subset) is an invariant on top of this single round-trip.

**Independent Test**: Run an embedded NATS server. Register a plain `nc.Subscribe("knowledge.recall", responder)` handler that echoes the request body. Construct an imp whose reasoning function calls `r.Request(ctx, "knowledge.recall", []byte("ping"))`. Drive a message through the imp; assert reasoning received `[]byte("ping")` (the echo) within a small bounded time and returned without error.

**Acceptance Scenarios**:

1. **Given** a reasoning function calls `Request` on a declared subject with no per-call timeout, **When** a responder replies within the harness's configured default request timeout, **Then** the call returns the response payload and no error.
2. **Given** a reasoning function calls `Request` with a per-call timeout, **When** the responder replies within that timeout, **Then** the call returns the response and the harness's default timeout is not consulted.
3. **Given** the resolved subject has no responder, **When** the call is made, **Then** the call returns `*ErrNoResponders{Subject: <resolved>}` before any reply deadline elapses (the substrate signals this via `nats.ErrNoResponders`).

---

### User Story 2 - Reasoning fans out via RequestMany (Priority: P1)

A developer's reasoning function invokes `r.RequestMany(ctx, "health.ping", nil, WithRequestManyWindow(200*time.Millisecond))`. The harness publishes a single request and collects every reply that arrives within the configured window (or up to a configured max-replies cap, whichever fires first). The call returns the collected replies as a slice.

**Why this priority**: Fan-out is a legitimate reasoning operation (health surveys, multi-responder coordination, broadcast queries). It is structurally unbounded — there is no upper bound on responder count, and the window has to elapse before the call returns. This is why it is reasoning-only.

**Independent Test**: Register three plain `nc.Subscribe("health.ping", ...)` handlers that each reply with their own ID. Construct an imp whose reasoning calls `r.RequestMany(ctx, "health.ping", nil, WithRequestManyWindow(200*time.Millisecond))`. Assert reasoning received exactly three replies (in any order) and that the call returned within `200ms + ε`.

**Acceptance Scenarios**:

1. **Given** N responders subscribe to a subject and reasoning calls `RequestMany` with a window `T_w` and no max-replies cap, **When** `T_w` elapses, **Then** the call returns all replies received during the window (order is substrate-determined; the harness does not sort).
2. **Given** N responders subscribe and reasoning calls `RequestMany` with `WithRequestManyMax(k)` where `k < N`, **When** the k-th reply arrives, **Then** the call returns the k replies immediately (the window is not waited out).
3. **Given** no responder exists, **When** reasoning calls `RequestMany`, **Then** the call returns an empty slice and no error after the window elapses. (Distinguishing "nobody listening" from "everybody listening but nobody replied" is not the framework's job; both are operationally indistinguishable to fan-out callers.)

---

### User Story 3 - Awareness reaches a service via Request (Priority: P1)

A developer's awareness function invokes `a.Request(ctx, "embed.short", payload, WithRequestTimeout(50*time.Millisecond))`. The harness issues a single NATS request on `embed.short` and returns the response payload. The awareness function uses the response to decide its verdict and yields back to the dispatch loop.

**Why this priority**: This is the structural energy gradient: awareness is allowed to do a single round-trip request/reply (bounded by call shape) but not fan-out and not fire-and-forget publish. Without `Request` on awareness, the bounded-capability use case (embed, short classification) cannot live in awareness at all — which collapses the gradient into a binary "anything goes / nothing goes" distinction.

**Independent Test**: Register a plain `nc.Subscribe("embed.short", ...)` handler that returns a small deterministic transformation of the request. Construct an imp whose awareness function calls `a.Request(ctx, "embed.short", payload)` and uses the response to choose `Wake` vs `Ignore`. Drive a message through; assert awareness received the response within the configured awareness-call timeout and that the verdict reflected the response.

**Acceptance Scenarios**:

1. **Given** a responder replies promptly to a subject, **When** awareness calls `Request` with a per-call timeout that exceeds the round-trip, **Then** the response is returned to awareness before the awareness function yields its verdict.
2. **Given** the per-call timeout is shorter than the round-trip latency, **When** the timeout elapses, **Then** the call returns `*ErrRequestTimeout{Subject: <resolved>, Timeout: <effective>}` and awareness can still yield a verdict (typically a `Note` recording the degraded state).

---

### User Story 4 - Awareness cannot RequestMany, Publish, or Conn; compile-time absence (Priority: P1)

The same awareness code attempting to invoke `RequestMany`, `Publish`, or `Conn` fails to compile because those methods do not exist on the `AwarenessContext` interface. Reasoning code can invoke any of them.

**Why this priority**: This is the constitutional guarantee. The energy gradient is enforced **structurally**, not by convention or runtime check. A developer who tries to fan out, publish, or grab the raw NATS connection from awareness writes code that does not compile. This is the same shape of guarantee `001-harness-core` delivers for `Publish` — extended here to `RequestMany` and (consistent with the 001 v2.2.0 cleanup that introduced `ReasoningContext.Conn()`) to `Conn`.

**Independent Test**: (a) Author a compile-time test (build-tagged file) whose body invokes `awareness.RequestMany(...)`; assert the file fails to compile under that tag. (b) Author a compile-time test (build-tagged file) whose body invokes `awareness.Publish(...)`; assert the file fails to compile (already covered by `001-harness-core`'s `awareness_no_publish.go`; this feature reaffirms it). (c) Author a compile-time test (build-tagged file) whose body invokes `awareness.Conn()`; assert the file fails to compile under that tag. (d) Construct an imp whose reasoning function invokes `RequestMany`, `Publish`, and `Conn()`; assert all succeed.

**Acceptance Scenarios**:

1. **Given** awareness code calls `a.RequestMany(...)`, **When** the code is compiled, **Then** compilation fails because `AwarenessContext` does not declare `RequestMany`.
2. **Given** awareness code calls `a.Publish(...)`, **When** the code is compiled, **Then** compilation fails (re-asserted from 001-harness-core).
3. **Given** awareness code calls `a.Conn()`, **When** the code is compiled, **Then** compilation fails because `AwarenessContext` does not declare `Conn` (the raw-connection escape hatch is reasoning-only).
4. **Given** reasoning calls `RequestMany` with at least one responder, **When** the window elapses, **Then** reasoning receives the collected replies and no error.

---

### User Story 5 - Error categories are NATS-native and distinct (Priority: P2)

A request/reply call can fail in two well-defined ways: no responder ever received the request (typically misconfiguration or all responders down), or the effective timeout elapsed before a reply arrived. Each failure returns a distinct typed error. An imp's code can pattern-match on the category to adapt behavior.

**Why this priority**: Without distinct error categories, every failure looks the same and the imp can only do one thing about all of them. The framework keeps the categories minimal — exactly two — because NATS itself draws this line and we honor it. Per-application-layer errors (a responder replying "I refuse" via headers or payload) are the responder's protocol, not the framework's category.

**Independent Test**: Drive two scenarios against an embedded NATS server: (a) `Request` a subject with no subscribers — expect `*ErrNoResponders`; (b) `Request` a subject whose handler is configured to delay past the per-call timeout — expect `*ErrRequestTimeout`. Each assertion checks the error category is the specific one, distinguishable via `errors.As`.

**Acceptance Scenarios**:

1. **Given** a `Request` (or `RequestMany`) targeting a subject with no live handler in the resolved surface, **When** the substrate reports `nats.ErrNoResponders`, **Then** the harness returns `*ErrNoResponders{Subject: <resolved>}`.
2. **Given** a `Request` whose responder does not reply within the effective timeout, **When** the deadline elapses, **Then** the harness returns `*ErrRequestTimeout{Subject: <resolved>, Timeout: <effective>}`. `errors.Is(err, context.DeadlineExceeded)` MUST succeed (the typed error wraps the deadline cause).
3. **Given** the caller's `ctx` is cancelled while a `Request` is in flight (e.g., shutdown), **When** the cancellation propagates, **Then** the call returns an error for which `errors.Is(err, context.Canceled)` succeeds. (Not a separate category; standard Go cancellation.)

---

### User Story 6 - Per-call timeout override; no harness retry (Priority: P2)

A developer's `Request` / `RequestMany` call may supply a per-call timeout (or window) that overrides the harness's configured default for that call. If the call exceeds the per-call value, the surface returns the corresponding error. The framework imposes no retry policy, no backoff, no circuit-breaker.

**Why this priority**: Policy-free is a constitutional choice — different imps need different recovery semantics, and the framework cannot guess which. P2 because Story 5 must land first to make timeout meaningful as a distinct error category.

**Independent Test**: Subscribe a handler that delays its reply by 200ms. Call `Request` twice: once with the default timeout (long enough to succeed) and once with `WithRequestTimeout(50*time.Millisecond)`. Assert: the first call returns success; the second returns `*ErrRequestTimeout`. Assert no second request was published by the harness in response to the timeout (no automatic retry).

**Acceptance Scenarios**:

1. **Given** a `Request` with no per-call timeout, **When** the call is made, **Then** the surface uses the harness's configured default request timeout.
2. **Given** a `Request` with `WithRequestTimeout(T)`, **When** the call is made, **Then** the surface enforces `T` for that call regardless of the configured default.
3. **Given** a `Request` returns `*ErrRequestTimeout`, **When** the call has returned, **Then** the harness MUST NOT publish any further request on the same subject in response to the failure (no automatic retry).
4. **Given** a `RequestMany` with no per-call window, **When** the call is made, **Then** the surface uses the harness's configured default request-many window.
5. **Given** a `RequestMany` with `WithRequestManyMax(k)`, **When** `k` replies have arrived, **Then** the surface returns immediately without waiting for the remainder of the window.

---

### User Story 7 - Subjects are literal (Priority: P2)

`Request`, `RequestMany`, and `Publish` send on the declared subject verbatim. The framework imposes no prefix, no platform-mode segment, no transformation. Per the constitution's "Imps see one subject path" principle (v2.2.0), the declared subject is the wire subject. Cross-account routing is configured at the substrate via NATS account imports.

**Why this priority**: Without literal-subject semantics, an imp's outbound surface would have a "what you declare ≠ what gets published" gap that surprises both authors and operators. P2 because Stories 1–5 establish the surface; the literal-subject guarantee is structural and asserted by capturing on-the-wire subjects in tests.

**Independent Test**: Run an imp with no subject-prefix option. Subscribe a substrate-side listener on the literal subject `knowledge.recall`. Call `r.Request(ctx, "knowledge.recall", payload)` from reasoning. Assert the substrate-side listener received the request on `knowledge.recall`, with no other transformation. Repeat with `Publish` and `RequestMany` — same guarantee.

**Acceptance Scenarios**:

1. **Given** reasoning calls `Request` / `RequestMany` / `Publish` with declared subject `S`, **When** the call is made, **Then** the harness issues the substrate request on `S` verbatim.
2. **Given** a responder lives in a different NATS account, **When** the operator configures an account import that maps the exported subject onto the imp's local subject `S`, **Then** the imp's `Request(ctx, "S", payload)` reaches the responder with no source-level cross-account awareness. (This is substrate behavior, not framework behavior; the framework's part of the contract is that it does not transform the subject.)

---

### Edge Cases

- **`Request` on a subject with multiple subscribers** — NATS delivers the request to one subscriber (or to one per queue group); `Request` returns the first reply received. Fan-out is `RequestMany`, not `Request`.
- **`RequestMany` where the responder count exceeds `WithRequestManyMax(k)`** — the call returns the first `k` replies and stops collecting. Subsequent replies are dropped on the harness's inbox unsubscribe.
- **`RequestMany` window of zero or negative** — config validation rejects a non-positive default window at `Run`; a non-positive `WithRequestManyWindow` per-call value is silently ignored (the harness default applies).
- **`Publish` on a subject the substrate's account ACLs forbid** — the substrate rejects the publish and the harness returns the substrate's error verbatim. No framework-side whitelist (per the 001 cleanup that landed alongside this feature).
- **`Request` from awareness on a subject already declared as a channel** — there is no framework-level conflict. The subject namespace is shared; the imp's design has to decide whether overlap is meaningful. The harness does not detect or warn about overlap.
- **The caller's `ctx` is cancelled while `Request` is in flight** — the call returns an error wrapping `context.Canceled`. The publish to NATS is best-effort cancelled.
- **A request payload's size exceeds the substrate's `MaxPayload`** — the substrate returns an error from `RequestWithContext`; the harness returns that error unwrapped (it is neither `*ErrNoResponders` nor `*ErrRequestTimeout` — the categories are exhaustive for *runtime* failures, but substrate misconfigurations remain visible). Operators get a clear log line; imp authors who care can `errors.Is(err, nats.ErrMaxPayload)`.
- **The substrate is disconnected at the moment of the call** — the substrate returns an error; same handling as the previous edge case. The harness does not buffer or retry.
- **`RequestMany` against a subject where one responder replies and one is slow** — the call returns either when the slow responder replies (within the window) or when the window elapses, whichever first; if `WithRequestManyMax(k)` is set and `k` already arrived, the call returns immediately.
- **A reasoning `Publish` on a subject also used for `Request`** — there is no framework-level conflict; the operator's NATS topology decides what that means.

## Requirements *(mandatory)*

### Functional Requirements

#### Awareness outbound surface

- **FR-101**: The `AwarenessContext` (defined in `001-harness-core`) MUST be extended with a `Request(ctx context.Context, subject string, payload []byte, opts ...RequestOption) ([]byte, error)` method. The method MUST issue a single NATS request on the literal declared subject, await a reply within the effective timeout, and return the reply bytes or one of the documented error categories.
- **FR-102**: The `AwarenessContext` MUST NOT expose a `RequestMany` method. The compile-time absence is the structural enforcement of "awareness does not fan out" (call-shape boundedness).
- **FR-103**: The `AwarenessContext` MUST NOT expose a `Publish` method. (Re-asserted from `001-harness-core` FR-014; this feature does not add one.)
- **FR-103b**: The `AwarenessContext` MUST NOT expose a `Conn` method. The raw-`*nats.Conn` escape hatch added to `ReasoningContext` in the `001-harness-core` v2.2.0 cleanup is reasoning-only; awareness cannot bypass the bounded surface by reaching for the underlying connection. The compile-time absence is the structural enforcement.

#### Reasoning outbound surface

- **FR-104**: The `ReasoningContext` (defined in `001-harness-core`) MUST be extended with `Request(ctx, subject, payload, opts...) ([]byte, error)` and `RequestMany(ctx, subject, payload, opts...) ([][]byte, error)`. `Request` is single round-trip; `RequestMany` collects replies within the effective window (and respects an optional max-replies cap).
- **FR-105**: The `ReasoningContext.Publish` method MUST publish the payload on the declared subject verbatim. Subject permissioning is the substrate's concern (NATS account ACLs); the framework imposes no whitelist on `Publish`, `Request`, or `RequestMany`.

#### Request semantics

- **FR-106**: `Request` MUST: (a) publish a NATS request on the declared subject verbatim with the payload bytes, (b) await a reply within the effective timeout (per-call override or configured default), (c) return the reply payload bytes or a typed error.
- **FR-107**: A `Request` MUST distinguish, at the error level, two failure categories: `*ErrNoResponders{Subject}` (substrate reports `nats.ErrNoResponders`) and `*ErrRequestTimeout{Subject, Timeout}` (no reply within the effective timeout). Both MUST satisfy `errors.Is` / `errors.As`. `*ErrRequestTimeout` MUST unwrap to `context.DeadlineExceeded`.
- **FR-108**: A `Request` MUST honor the caller's `ctx`. When `ctx` is cancelled, the call MUST return promptly with an error for which `errors.Is(err, context.Canceled)` succeeds.
- **FR-109**: The framework MUST NOT impose retries, exponential backoff, or circuit-breaker policy on `Request`. A failed call returns; the imp's code decides what to do next.

#### RequestMany semantics

- **FR-110**: `RequestMany` MUST: (a) subscribe a temporary inbox, (b) publish a NATS request on the declared subject verbatim with the inbox as reply subject, (c) collect every reply that arrives within the effective window (per-call override or configured default), (d) return the collected replies as a slice (or an empty slice if none arrived). The reply-message order is substrate-determined; the harness does not sort.
- **FR-111**: `RequestMany` MUST honor a per-call `WithRequestManyMax(k)` cap. When `k` replies have arrived, the call MUST return immediately without waiting for the remainder of the window.
- **FR-112**: `RequestMany` MUST honor the caller's `ctx`. When `ctx` is cancelled, the call MUST return promptly; whatever replies have been collected so far MAY be returned alongside the cancellation error (the call MAY also return a nil slice — both are acceptable v1 behaviors).
- **FR-113**: `RequestMany` MUST clean up its temporary inbox subscription regardless of how it returns (success, timeout, cancel). No inbox leak on any path.
- **FR-114**: The framework MUST NOT impose retries on `RequestMany`. If zero replies arrived, that is the outcome; the imp decides whether to call again.

#### Subjects

- **FR-115**: `Request`, `RequestMany`, and `Publish` MUST send on the declared subject verbatim, consistent with `001-harness-core` FR-030 and the constitution's "Imps see one subject path" principle (v2.2.0). Cross-account access is configured at the substrate via NATS account imports.

#### Options

- **FR-116**: The harness MUST expose two new construction options: `WithDefaultRequestTimeout(d time.Duration)` (default 5s) and `WithDefaultRequestManyWindow(d time.Duration)` (default 1s). Non-positive values MUST fail config validation at `Run` with `*ErrConfigInvalid` (the existing `001-harness-core` error shape).
- **FR-117**: The harness MUST expose per-call options: `WithRequestTimeout(d time.Duration)` for `Request`, and `WithRequestManyWindow(d time.Duration)` plus `WithRequestManyMax(n int)` for `RequestMany`. Non-positive per-call values MUST be silent no-ops (the harness default applies instead of failing the call).

#### Errors

- **FR-118**: All errors introduced by this feature MUST be exported sentinel types matching the `001-harness-core` error convention (typed, satisfying `errors.Is`/`errors.As`, naming the offending value in `Error()`). New error types:
  - `*ErrNoResponders{Subject string}` — substrate reports no handler for the subject.
  - `*ErrRequestTimeout{Subject string; Timeout time.Duration}` — call exceeded effective timeout. `Unwrap()` returns `context.DeadlineExceeded`.
- **FR-119**: Extensions to `*ErrConfigInvalid` (non-positive default request timeout or default request-many window) MUST follow the existing `*ErrConfigInvalid{Field, Reason}` shape (`001-harness-core` FR-136).

#### Observability

- **FR-120**: The `Metrics` snapshot (`001-harness-core`) MUST be extended with at least:
  - `RequestCalls uint64` — total `Request` invocations (success + failure).
  - `RequestManyCalls uint64` — total `RequestMany` invocations.
  - `RequestNoResponders uint64` — total `Request` (and `RequestMany`) failures with `*ErrNoResponders`.
  - `RequestTimeouts uint64` — total `Request` failures with `*ErrRequestTimeout`. (`RequestMany` does not produce timeouts — it ends with whatever it collected; an empty slice is the "all timed out" signal.)

#### Out of scope (explicit non-requirements)

- **FR-NS-101**: This feature does NOT introduce a capability-declaration section on `ImpSpec`. The imp talks to subjects; what is on the other end is the operator's design.
- **FR-NS-102**: This feature does NOT introduce `$SRV.INFO` discovery, endpoint-metadata aggregation, or a resolved-surface map. Imps name subjects directly.
- **FR-NS-103**: This feature does NOT introduce retry, backoff, or circuit-breaker behavior. Calls are one-shot; recovery is imp responsibility.
- **FR-NS-104**: This feature does NOT introduce a JSON-marshaling generic helper at the framework level. Imps that want typed payloads bring their own codec (`encoding/json`, `proto`, anything). The framework's surface is bytes in, bytes out — the same as `Publish`.
- **FR-NS-105**: This feature does NOT introduce service-error decoding (e.g., NATS micro's `Nats-Service-Error` header). A responder that signals an application error via headers or payload does so under its own protocol; the imp's code interprets that. From the framework's perspective, that reply is a successful `Request` return.
- **FR-NS-106**: This feature does NOT introduce streaming responses (multiple replies forming a logical stream terminated by a sentinel). `RequestMany` is "collect within window"; streaming is a future enhancement.
- **FR-NS-107**: This feature does NOT introduce queue-group or sharding policy. Multiple responder instances are handled by the substrate's queue-group semantics; the client surface is unaware.
- **FR-NS-108**: This feature does NOT introduce per-call request headers (`nats.Header`). The byte-level surface is symmetric with `Publish` (`001-harness-core` does not expose headers on publish either). Headers are a future enhancement when a use case demands them.

### Key Entities

- **Request Call** — A single request/reply round trip on the declared subject (verbatim — no framework-side transformation). Returns the reply payload bytes or a typed error. Bounded by the effective timeout. Available on both `AwarenessContext` and `ReasoningContext`.
- **Request-Many Call** — A fan-out request/reply: one publish on the declared subject, many replies collected within a window (or up to a cap). Returns a slice of reply payloads. Available on `ReasoningContext` only.
- **Effective Timeout / Window** — The actually-applied bound on a call: the per-call override if positive, otherwise the harness's configured default.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-101**: A reasoning function calling `r.Request("knowledge.recall", payload)` against a local responder receives the reply within a small bounded time. (Target round-trip: under 100 ms on a clean local embedded NATS server.)
- **SC-102**: A reasoning function calling `r.RequestMany("health.ping", nil, WithRequestManyWindow(200*time.Millisecond))` against three local responders receives three replies and returns within `200ms + small ε`.
- **SC-103**: An awareness function calling `a.Request("embed.short", payload)` receives the reply within the awareness-call timeout budget. The invoke completes before awareness yields its verdict. (Target round-trip: under 100 ms in a clean local embedded NATS environment.)
- **SC-104**: A compile-time check confirms the `AwarenessContext` type does not expose `RequestMany`, `Publish`, or `Conn`. Each forbidden method has its own build-tagged file under `integration/compiletest/`; the CI gate runs `go vet -tags=<tag>` for each and asserts non-zero exit. (Parallel to `001-harness-core` SC-006 for `Publish`.)
- **SC-105**: Each of the two error categories (`ErrNoResponders`, `ErrRequestTimeout`) is produced and matched in tests against controlled subscriber configurations; no category collapses into a generic error.
- **SC-106**: A per-call timeout override shorter than the harness default and shorter than the responder's controlled-delay returns `*ErrRequestTimeout` within the override window (target: actual elapsed within `T + small ε`), and the harness does not publish a retry request.
- **SC-107**: An imp that calls neither `Request` nor `RequestMany` nor `Publish` (a pure-awareness imp doing only state updates) runs identically to a `001-harness-core` imp — no extra metrics counters move beyond the existing ones; no extra substrate traffic.

## Assumptions

- **NATS request/reply semantics are the framework's outbound model.** `nc.RequestWithContext` for `Request`; subscribe-inbox-then-publish for `RequestMany`. The framework does not reinvent the wire.
- **No central registry, no discovery, no $SRV.INFO.** Constitutional non-negotiable; reinforced by the principle "Imps see one subject path" (constitution v2.2.0). Imps know the subjects they need; operators ensure those subjects have responders.
- **The harness consumes service-side application errors transparently.** A responder that signals an application-level error (via headers, payload structure, etc.) returns a successful `Request` from the framework's perspective. Imp code interprets the payload.
- **The harness uses the imp's existing NATS connection** (the connection passed to `NewImp` in `001-harness-core`) for `Request`, `RequestMany`, and `Publish`. This feature does not introduce a separate connection pool.
- **The single-subject-path principle holds.** Imps source single-form, literal subjects; the framework imposes no transformation. Cross-account routing is the operator's concern (NATS account imports), not the framework's. Constitution v2.2.0.
- **The legacy codebase at `../imps-legacy`** has prior implementations of NATS micro client code and request/reply patterns. It MAY be consulted for implementation prior-art (e.g., how subscribe-inbox-then-publish was structured for collect-with-window semantics in `catalog/tools/natsmicro/`) but is NOT the source of architecture for this feature. The new awareness/reasoning split has no analog in the legacy code; design from this spec.
