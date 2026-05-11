# Phase 0 Research: Request/Reply Surface

This document records the design decisions made before Phase 1 design. Each
entry follows: **Decision**, **Rationale**, **Alternatives considered**.

The spec arrived without `NEEDS CLARIFICATION` markers. This feature
layers on top of `001-harness-core`. Decisions there (Go 1.25, `nats.go`,
embedded server for tests, `slog`, `internal/` layout, lifecycle state
machine, error-type convention, and — after constitution v2.2.0 — the
literal-subject discipline with no resolver) are inherited; this document
only records what is new or different.

---

## R-101: Surface — Request, RequestMany, Publish on the contexts

**Decision**: Three outbound primitives, partitioned across the two
context types:

```go
type AwarenessContext interface {
    State(name string, entity Entity) (StateRef, error)
    Request(ctx context.Context, subject string, payload []byte, opts ...RequestOption) ([]byte, error)
}

type ReasoningContext interface {
    State(name string, entity Entity) (StateRef, error)
    InFlight() int
    Publish(ctx context.Context, subject string, payload []byte) error  // verbatim subject, no whitelist
    Conn() *nats.Conn                                                    // escape hatch (added in 001 v2.2.0 cleanup)
    Request(ctx context.Context, subject string, payload []byte, opts ...RequestOption) ([]byte, error)
    RequestMany(ctx context.Context, subject string, payload []byte, opts ...RequestManyOption) ([][]byte, error)
}
```

The energy gradient is enforced **by call shape on each context type**:

- `Request` is a single round trip (one publish, one reply, one deadline)
  — structurally bounded; awareness can do it.
- `RequestMany` is fan-out (one publish, many replies, a collection
  window) — structurally unbounded; reasoning-only.
- `Publish` is fire-and-forget (one publish, no reply, side-effecting)
  — reasoning-only (unchanged from 001).

The compile-time absence of `RequestMany` and `Publish` on
`AwarenessContext` is the structural enforcement. A build-tagged file
under `integration/compiletest/` asserts that
`awareness.RequestMany(...)` does not compile (parallel to 001's
`awareness_no_publish.go`).

**Rationale**: NATS users already recognize `Request` / `RequestMany` /
`Publish`. The framework adds no new vocabulary. The energy gradient
maps cleanly onto call shape: "is this a single bilateral round trip"
is exactly the boundedness criterion the constitution names. No
metadata-driven boundedness flag is needed — the property is in the
call site itself.

This replaces an earlier draft of this feature that introduced
`InvokeBoundedCapability` / `InvokeCapability` methods on the contexts
plus a `Capabilities` declaration on `ImpSpec` plus a `$SRV.INFO`
discovery flow plus an `imp.bounded` metadata key. That design imposed
framework ceremony where NATS primitives already suffice; the new
design is materially smaller and more honest about what the framework
actually does.

**Alternatives considered**:

- A single context type with a runtime "this is awareness, that call is
  unbounded, refuse" check — rejected; collapses the structural
  guarantee into a policy check, contrary to constitution v2.1.0
  "The energy gradient is structural".
- Generic typed helpers (`Invoke[Req, Resp]` etc.) at the framework
  level — rejected for v1; the framework's surface is bytes in, bytes
  out (symmetric with `Publish`), and imps that want typed payloads
  bring their own codec. JSON-marshaling at the framework layer would
  be a per-capability concern.
- Renaming `Request`/`RequestMany` to NAT-non-native names
  (`InvokeOne`, `Survey`, `Broadcast`, etc.) — rejected; the spec
  explicitly prefers the names NATS users already know.

---

## R-102: Subjects are literal (constitution v2.2.0)

**Decision**: `Request`, `RequestMany`, and `Publish` all send on the
declared subject verbatim. The framework performs no prefix-insertion,
no platform-mode segment, no subject rewriting. Channels behave the
same way (001 cleanup landed alongside this feature).

When a responder lives in a different NATS account, an operator-
configured **NATS account import** maps the exported subject onto
whatever local name the imp's account uses. The imp's source and the
harness see only the imp-account subject — verbatim.

**Rationale**: Constitution v2.2.0 refined the "Imps see one subject
path" principle from "single-form `<prefix>.<declared>`" to "the
framework imposes no transformation." The earlier prefix-insertion
rule produced unpredictable behavior ("I configured X but the imp
publishes on `whatever.X`") and was redundant with NATS account-level
scoping that an operator already has to configure for multi-tenant
deployments. Removing the framework-side prefix collapses the matrix.

**Alternatives considered**:

- Keeping an optional prefix (default empty) at the framework level —
  rejected; the surprise factor is in the option *existing*, not in its
  default. Substrate scoping is where this concern belongs.
- Re-introducing platform-mode for capability calls only — rejected;
  contradicts the principle.

---

## R-103: Dispatch for Request

**Decision**: A `Request` invocation:

1. Computes the effective timeout: per-call `WithRequestTimeout(d)` if
   `d > 0`, else the harness's `defaultRequestTimeout`.
2. Derives a context via `context.WithTimeout(ctx, effective)` from the
   caller's `ctx` — so the request honors both the caller's
   cancellation and the effective timeout, whichever fires first.
3. Calls `nc.RequestWithContext(derivedCtx, subject, payload)` on the
   literal declared subject.
4. Translates the outcome:
   - reply received → return `msg.Data, nil`.
   - `errors.Is(err, nats.ErrNoResponders)` → `*ErrNoResponders{Subject: subject}`.
   - `errors.Is(err, context.DeadlineExceeded)` AND the derived context's
     deadline elapsed (not the caller's) → `*ErrRequestTimeout{Subject: subject, Timeout: effective}`.
   - `errors.Is(err, context.Canceled)` from the caller's context →
     return the error (preserves `errors.Is(err, context.Canceled)`).
   - Any other error → return unwrapped (substrate-specific, rare; e.g.,
     `nats.ErrMaxPayload`, connection drop). Visible to log + `errors.Is`.

No retry. No backoff. No second NATS publish on failure (FR-109,
SC-106).

**Rationale**: `nc.RequestWithContext` already does the
publish-then-await-reply work; the framework's job is to translate
substrate signals into the two typed categories the spec defines. The
"derived context" trick is what `001-harness-core`'s `Publish` already
does for cancellation; we extend it with a deadline.

**Alternatives considered**:

- Implementing the request/reply by hand (subscribe inbox, publish,
  select) — rejected; reinvents `nc.RequestWithContext` and is more
  surface area for bugs.
- Returning `context.DeadlineExceeded` directly instead of wrapping
  in `*ErrRequestTimeout` — rejected; SC-105 requires distinct typed
  categories. The wrapped error preserves `errors.Is(err,
  context.DeadlineExceeded)` for callers who don't care about the
  typed wrapper.

---

## R-104: Dispatch for RequestMany

**Decision**: A `RequestMany` invocation:

1. Computes the effective window: per-call `WithRequestManyWindow(d)`
   if `d > 0`, else `defaultRequestManyWindow`.
2. Reads `WithRequestManyMax(k)` — zero or negative means "no cap;
   collect for the full window".
3. Creates an inbox via `nc.NewRespInbox()` and subscribes a buffered
   channel: `nc.ChanSubscribe(inbox, replyCh)` with capacity sized to
   `max(k, 64)`.
4. Publishes the request on the literal declared subject:
   `nc.PublishMsg(&nats.Msg{Subject: subject, Reply: inbox, Data: payload})`.
5. Collects replies in a select loop until either (a) `k` replies have
   arrived (when `k > 0`), (b) the window deadline elapses, (c) the
   caller's `ctx` is cancelled.
6. Unsubscribes the inbox via `sub.Unsubscribe()` regardless of how
   the loop exits. This MUST happen in every path (success, window
   timeout, cancellation) — FR-113.
7. Returns the collected reply payloads as `[][]byte`, plus:
   - on caller-context cancellation: the partial slice plus an error
     wrapping `context.Canceled` (FR-112; either-partial-or-empty is
     allowed by v1).
   - on window elapse: the collected slice (possibly empty) and `nil`
     error (FR-114).
   - on substrate-publish failure: nil slice and the substrate error.

No retry. Empty slice is a valid "all responders absent" return.

**Rationale**: This is the canonical "fan-out collect" pattern in
NATS Go. The buffered channel capacity matters — too small and slow
collection backpressures the substrate; too large and idle inboxes
waste memory. Using `max(k, 64)` is a pragmatic default: bounded by
the cap when set, generous otherwise.

The "empty slice is valid" return contract is the key simplification —
the spec deliberately collapses "no responders" and "responders but
none replied" into the same outcome for fan-out (US-2 acceptance
scenario 3). Distinguishing them is operator concern, not framework
concern.

**Alternatives considered**:

- Returning an error when zero replies arrive — rejected; gives
  fan-out caller code two paths to handle (slice-empty vs error)
  for the same observable. Empty-slice-only is simpler.
- Returning typed `*Reply` records (with `Subject`, `Headers`,
  `Data`) instead of `[]byte` payloads — rejected for v1 symmetry with
  `Request` and `Publish` (both byte-shaped). If headers become
  load-bearing later, the same `WithRequestHeaders(...)` extension can
  carry them on both sides.
- Implementing as a generator / iterator (channel-of-replies returned
  to the caller) — rejected for v1; the collect-and-return shape is
  simpler. An iterator can be added later as a separate method
  (`RequestStream`?) when a use case demands it (likely inference
  streaming).

---

## R-105: Error categories — minimal, NATS-native

**Decision**: Two typed errors:

```go
type ErrNoResponders struct{ Subject string }
type ErrRequestTimeout struct{ Subject string; Timeout time.Duration }

func (e *ErrRequestTimeout) Unwrap() error { return context.DeadlineExceeded }
```

`Request` returns one of:

- the reply payload (success)
- `*ErrNoResponders` (no handler)
- `*ErrRequestTimeout` (deadline elapsed)
- `context.Canceled`-wrapped (caller cancelled)
- substrate-specific (e.g., `nats.ErrMaxPayload`) — passed through.

`RequestMany` returns one of:

- a slice of replies, possibly empty (success or window elapsed)
- partial slice + `context.Canceled`-wrapped (caller cancelled)
- nil slice + substrate-specific error (publish failed).

`*ErrNoResponders` is **not** returned from `RequestMany` in the fan-out
path — the "nobody home" condition there is the empty slice (FR-110,
US-2 AS-3). The one narrow exception is a substrate-level publish refusal:
if NATS reports `nats.ErrNoResponders` for the *publish itself* (e.g.,
account permissions reject the subject before any responder is consulted),
the harness wraps that as `*ErrNoResponders` and increments
`RequestNoResponders` — see R-111. This is a publish-side failure, not a
fan-out outcome.

**Rationale**: NATS itself draws the line at these two cases. Any
application-layer error (a responder replying "I don't understand"
or "internal failure" via payload structure) is the responder's
protocol — the framework does not parse it. This stays true to the
constitution: "No generic capability protocol".

The earlier draft of this feature distinguished four categories
(adding `*ErrCapServiceError` for NATS micro `Nats-Service-Error`
headers and `*ErrCapProtocolError` for decode failures). Both are
gone: NATS micro's error-header convention is a *capability service
protocol*, not a framework concern; decode failures belong to whatever
codec the imp chose (JSON, proto, etc.) and live in the imp's code,
not the framework's.

**Alternatives considered**:

- Returning `nats.ErrNoResponders` unwrapped — rejected; the typed
  wrapper carries the subject in the error, which is
  diagnostically useful and aligns with the 001-harness-core
  convention (`*ErrSubscriptionFailed{Subject, Cause}`, etc.).
- Adding a `*ErrSubstrateError{Cause}` umbrella category — rejected;
  pollutes the surface for the rare "something else went wrong"
  case. Bare error pass-through is fine — `errors.Is` works.

---

## R-106: Options — defaults and per-call overrides

**Decision**: Two new harness-construction options:

```go
func WithDefaultRequestTimeout(d time.Duration) Option       // default 5 s
func WithDefaultRequestManyWindow(d time.Duration) Option    // default 1 s
```

Three new per-call options:

```go
func WithRequestTimeout(d time.Duration) RequestOption
func WithRequestManyWindow(d time.Duration) RequestManyOption
func WithRequestManyMax(n int) RequestManyOption
```

Validation:

- Non-positive harness defaults → `*ErrConfigInvalid` at `Run`
  (consistent with 001 FR-136).
- Non-positive per-call values → silent no-op (the harness default
  applies). This is the "principle of least surprise" path; if a user
  passes `WithRequestTimeout(0)` thinking "no timeout", they get the
  harness default instead of an unbounded wait — far safer.
- `WithRequestManyMax(0)` or negative → treated as "no cap".

**Rationale**: Defaults follow the spec wording (FR-116). The per-call
"silent no-op" rule is what `001-harness-core` already does for
`WithCapTimeout` (the previously planned option) and keeps imp code
robust to "I forgot to compute the duration" bugs.

**Alternatives considered**:

- Validating per-call options at the call site (returning an error for
  non-positive values) — rejected; per-call validation is per-call
  ceremony that distracts from the work the imp's code is doing. The
  harness's default is the right answer for the misuse case.

---

## R-107: No framework whitelist; substrate ACLs gate everything

**Decision**: Neither `Request`, `RequestMany`, nor `Publish` is gated
by a framework-side whitelist. Subject permissioning is the substrate's
concern (NATS account ACLs on the imp's connection). This matches the
001-harness-core v2.2.0 cleanup that removed `ImpSpec.Actions` and
`*ErrWhitelistViolation`.

**Rationale**: A framework-side whitelist on outbound subjects was
defense-in-depth that didn't actually defend anything — the framework
runs the imp's code; if the imp can't be trusted, the imp can call
`nc.Publish` directly via the `ReasoningContext.Conn()` accessor and
skip any framework check. NATS account ACLs are an out-of-process
check that actually constrains a compromised process. That's the right
mechanism. The framework gets out of the way.

**Alternatives considered**:

- Keeping `Actions` for `Publish` only (the previous design) —
  rejected in the v2.2.0 cleanup; defense-in-depth that depends on the
  controlled component being trustworthy isn't defense.
- A separate `ImpSpec.Requests` whitelist — rejected as ceremony.
- Unifying `Actions` to cover all outbound subjects — rejected; the
  unification with substrate ACLs is cleaner.

---

## R-108: No in-tree fixture; tests use raw NATS subscribers

**Decision**: This feature does NOT ship an in-tree fixture
(no `testutil/capfixture`). Integration tests register
`nc.Subscribe(...)` handlers directly in the test body. Each handler
is a small closure that returns whatever payload the test scenario
needs (echo, delayed, error-shaped, etc.).

**Rationale**: The earlier draft introduced a `testutil/capfixture`
package that registered a NATS micro service with bounded/unbounded
endpoints and controllable knobs (delay, service-error, malformed
reply). All of that was scaffolding for the capability-declaration
ceremony that this feature no longer has. With `Request`/`RequestMany`
the test pattern is just "subscribe and reply": a couple of lines in
the test body. A dedicated fixture would be overkill.

The existing `testutil/natstest` helper (embedded NATS server with
JetStream toggle) is sufficient.

**Alternatives considered**:

- Keeping a minimal helper that wires up echo-style responders for
  common patterns — rejected as premature. If tests start repeating
  the same six-line setup, we can extract a helper then.

---

## R-109: Compile-time guarantees — three build-tagged assertions

**Decision**: Three build-tagged files live under
`integration/compiletest/`, one per forbidden method on `AwarenessContext`:

- `awareness_no_publish.go` — already exists; asserts
  `awareness.Publish(...)` does not compile under tag
  `awareness_publish_must_fail`. Unchanged.
- `awareness_no_requestmany.go` — **new**; asserts
  `awareness.RequestMany(...)` does not compile under tag
  `awareness_requestmany_must_fail`.
- `awareness_no_conn.go` — **new**; asserts
  `awareness.Conn()` does not compile under tag
  `awareness_conn_must_fail`. The raw-`*nats.Conn` escape hatch added
  to `ReasoningContext` in the 001 v2.2.0 cleanup is reasoning-only;
  this file enforces that awareness cannot bypass the bounded surface
  by reaching for the underlying connection (FR-103b).

CI runs `go vet -tags=<tag> ./integration/compiletest/...` for each tag
and asserts a non-zero exit (the build failure is the assertion).

**Rationale**: Each method allowed on `ReasoningContext` but forbidden
on `AwarenessContext` deserves its own compile-time test. The pattern
is mechanical; one file per forbidden method. A fourth forbidden method
in the future would get its own file too.

**Alternatives considered**:

- One file asserting multiple forbidden methods in one body — rejected;
  if one of the methods is accidentally added to `AwarenessContext`,
  the file still fails to compile for the other forbidden method,
  masking the regression. One method per file gives one clear failure
  per regression.

---

## R-110: Internal package layout

**Decision**: No new internal sub-package. The new methods live
directly in `harness/`:

- `harness/context.go` — interface declarations extended.
- `harness/context_awareness.go` — concrete `Request` method.
- `harness/context_reasoning.go` — concrete `Request`, `RequestMany` methods.
- `harness/options.go` — `WithDefaultRequestTimeout`,
  `WithDefaultRequestManyWindow` added to `runtimeOptions`.
- `harness/request.go` — **new file** holding shared dispatch code
  (effective-timeout computation, error translation) used by both
  context types. Package-level helpers, not exported.
- `harness/errors.go` — `ErrNoResponders`, `ErrRequestTimeout` added.

No `internal/request/` or `internal/capability/` sub-package. The
implementation is small enough — a few hundred lines including
tests — that an internal sub-package would be ceremony.

**Rationale**: The earlier capability-client design added
`internal/capability/` with `surface.go`, `discovery.go`, `invoke.go`,
and a fixture sub-package. All of that disappears with the new design.
What's left is a few methods and one helper file; that fits in
`harness/` next to `context_reasoning.go`.

The constitution's "imps stay small and agile" commitment applies to
the framework's internal structure too: don't create packages for the
sake of packages.

**Alternatives considered**:

- Folding the implementation into the context concrete-type files
  (`context_awareness.go`, `context_reasoning.go`) and duplicating
  the dispatch logic — rejected; the dispatch is identical between
  awareness's `Request` and reasoning's `Request`, and DRY wins here.
- Carving `internal/request/` even though the surface is tiny —
  rejected; ceremony.

---

## R-111: Observability counters

**Decision**: Four new counters on `harness.Metrics`:

- `RequestCalls uint64` — total `Request` invocations (success + failure).
- `RequestManyCalls uint64` — total `RequestMany` invocations.
- `RequestNoResponders uint64` — `Request` and `RequestMany` failures
  that returned `*ErrNoResponders` (the latter is rare — `RequestMany`
  returns empty slice, not `*ErrNoResponders` — but if the substrate
  refuses the publish entirely, this counter still increments).
- `RequestTimeouts uint64` — `Request` failures with
  `*ErrRequestTimeout`.

`Metrics.snapshot()` is extended to read these fields.

`RequestMany` does NOT have a "windows elapsed" counter — the elapsed
state is just a successful call returning an empty (or partial) slice.

**Rationale**: Two universal counters (calls per kind) plus two
failure-mode counters give operators enough granularity to dashboard
without bloating the surface. Per-subject counters are deliberately
omitted; logs cover that need (R-118 of the previous research, now
historical, made the same argument).

**Alternatives considered**:

- A single `RequestFailures` umbrella counter — rejected; the two
  failure modes are different enough operationally (no-responders →
  often misconfiguration; timeout → usually load) that they warrant
  separate visibility.
- Per-subject histograms — rejected as bloat. Operator-side
  observability tools can derive per-subject metrics from logs.

---

## R-112: No JSON or other codec at the framework level

**Decision**: `Request` and `RequestMany` take and return `[]byte`.
The framework imposes no codec. Imps that want typed payloads
serialize before the call and deserialize after — using whatever
codec their responder agreed on.

**Rationale**: Constitution non-negotiable "No generic capability
protocol". Inference will stream, knowledge may use proto, tool
execution will use JSON. The framework cannot pick a codec without
imposing on at least one of them. Bytes in, bytes out is the
substrate-honest answer.

This is a divergence from the earlier draft, which exposed
`Invoke[Req, Resp any]` generic helpers with built-in JSON
marshal/unmarshal. Those go away. An imp that wants typed JSON
helpers writes:

```go
func myJSONRequest[Req, Resp any](ctx context.Context, r harness.ReasoningContext, subject string, req Req) (Resp, error) {
    payload, err := json.Marshal(req)
    if err != nil { return zero[Resp](), err }
    reply, err := r.Request(ctx, subject, payload)
    if err != nil { return zero[Resp](), err }
    var resp Resp
    return resp, json.Unmarshal(reply, &resp)
}
```

A handful of lines per imp. No framework ceremony.

**Alternatives considered**:

- Shipping a `harness/codec/json.go` sub-package with the helpers —
  rejected for v1; if every imp eventually copies the same six lines,
  we can extract then. But more likely: different imps will want
  different codecs, and standardizing on JSON would push imps the
  wrong way (NATS payloads aren't typically JSON-shaped beyond small
  request/reply pairs).

---

## Open questions deferred to subsequent features

Recording these for completeness; none block this feature:

- Streaming responses (multiple replies forming a logical stream
  terminated by a sentinel) — likely for inference; deferred to that
  feature.
- Per-call NATS headers on `Request` and `RequestMany` — deferred
  until a use case demands them.
- A `RequestIterator` (stream-of-replies) API for `RequestMany` —
  deferred; the collect-and-return shape is sufficient for v1.
- Subject-permission introspection at startup (asking the substrate
  whether the imp's connection can publish on its declared channel
  subjects) — defense-in-depth; deferred.
- JSON / proto / msgpack helpers — out of scope; per-imp choice.
- Retry-on-timeout helper wrappers — explicitly out of scope (FR-NS-103);
  imp code writes whatever retry policy it wants.
