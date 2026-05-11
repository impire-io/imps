# Phase 1 Data Model: Request/Reply Surface

This document records the entities and relationships this feature adds.
Each entity is described in terms of its fields, validation rules, and
(where relevant) lifecycle. Public-API types are flagged; everything
else is internal to the harness package.

The fields here are the *logical* shape — Go struct definitions in code
may differ in field order or unexported helpers. What this document
constrains is the contract.

This model extends — does not replace — the entities defined in
`001-harness-core/data-model.md`. Only new or modified entities appear
here.

---

## AwarenessContext *(public, interface, modified)*

| Method | Return | Notes |
|---|---|---|
| `State(name string, entity Entity) (StateRef, error)` | (unchanged) | (Unchanged from `001-harness-core`.) |
| `Request(ctx context.Context, subject string, payload []byte, opts ...RequestOption) ([]byte, error)` | reply bytes or typed error | **NEW**. Issues a single NATS request on the resolved subject. Returns the reply payload on success or one of `*ErrNoResponders`, `*ErrRequestTimeout`, or a `context.Canceled`-wrapping error. No retry. |

**Critical invariant** (extended from `001-harness-core`): there is **no**
`RequestMany`, **no** `Publish`, and **no** `Conn` method on
`AwarenessContext`. The compile-time absences are asserted by build-
tagged files under `integration/compiletest/` — one file per forbidden
method, so a regression that adds any one of them produces a single
clear build failure for that specific method:

- `awareness_no_publish.go` (unchanged from 001) — tag
  `awareness_publish_must_fail`.
- `awareness_no_requestmany.go` (new) — tag
  `awareness_requestmany_must_fail`.
- `awareness_no_conn.go` (new) — tag `awareness_conn_must_fail`.
  The raw-`*nats.Conn` escape hatch added to `ReasoningContext` in the
  001 v2.2.0 cleanup is reasoning-only; awareness cannot bypass the
  bounded surface by reaching for the underlying connection.

---

## ReasoningContext *(public, interface, modified)*

| Method | Return | Notes |
|---|---|---|
| `State(name string, entity Entity) (StateRef, error)` | (unchanged) | |
| `InFlight() int` | (unchanged) | |
| `Publish(ctx context.Context, subject string, payload []byte) error` | (unchanged from 001 v2.2.0 cleanup) | Publishes on the declared subject verbatim. No framework whitelist; substrate ACLs gate. |
| `Conn() *nats.Conn` | (added in 001 v2.2.0 cleanup) | Escape hatch for generic NATS-based clients used from reasoning. Not available on awareness. |
| `Request(ctx context.Context, subject string, payload []byte, opts ...RequestOption) ([]byte, error)` | reply bytes or typed error | **NEW**. Same semantics as `AwarenessContext.Request`. |
| `RequestMany(ctx context.Context, subject string, payload []byte, opts ...RequestManyOption) ([][]byte, error)` | collected replies or typed error | **NEW**. Issues a single NATS request, subscribes a temporary inbox, collects replies for the effective window (or up to the cap), returns the slice. Inbox is unsubscribed on every return path. |

The `ctx` cancellation continues to be wired to harness shutdown
(`001-harness-core` FR-126 analog); an in-flight `Request` or
`RequestMany` MUST return promptly with `context.Canceled`-wrapping
errors when shutdown begins.

---

## RequestOption *(public)*

A functional option type for `Request` calls.

```go
type RequestOption func(*requestOptions)

func WithRequestTimeout(d time.Duration) RequestOption
```

| Option | Notes |
|---|---|
| `WithRequestTimeout(d)` | Per-call timeout override. When `d > 0`, takes precedence over the harness's configured default (`WithDefaultRequestTimeout`). When `d <= 0`, the option is a silent no-op (the harness default applies). |

---

## requestOptions *(internal, populated by RequestOption)*

| Field | Type | Notes |
|---|---|---|
| `timeout` | `time.Duration` | Zero means "use harness default". Negative values are clamped to zero in `WithRequestTimeout`. |

---

## RequestManyOption *(public)*

A functional option type for `RequestMany` calls.

```go
type RequestManyOption func(*requestManyOptions)

func WithRequestManyWindow(d time.Duration) RequestManyOption
func WithRequestManyMax(n int) RequestManyOption
```

| Option | Notes |
|---|---|
| `WithRequestManyWindow(d)` | Per-call window override. When `d > 0`, takes precedence over the harness's default (`WithDefaultRequestManyWindow`). When `d <= 0`, silent no-op. |
| `WithRequestManyMax(n)` | Maximum number of replies to collect. When `n > 0`, the call returns as soon as `n` replies have arrived (without waiting for the rest of the window). When `n <= 0`, the call collects for the full window. |

---

## requestManyOptions *(internal, populated by RequestManyOption)*

| Field | Type | Notes |
|---|---|---|
| `window` | `time.Duration` | Zero means "use harness default". |
| `max`    | `int`           | Zero or negative means "no cap; collect for the full window". |

---

## runtimeOptions *(public via Options, modified)*

Extended with two new fields and two new builder functions.

| Field | Type | Default | Notes |
|---|---|---|---|
| (existing) | (as in 001-harness-core) | (unchanged) | After v2.1.0 cleanup: `drainWindow`, `logHandler`, `subjectPrefix` only. |
| `defaultRequestTimeout` | `time.Duration` | `5 * time.Second` | Default per-call timeout when `Request` does not supply `WithRequestTimeout`. Non-positive value → `*ErrConfigInvalid` at `Run`. |
| `defaultRequestManyWindow` | `time.Duration` | `1 * time.Second` | Default collection window when `RequestMany` does not supply `WithRequestManyWindow`. Non-positive value → `*ErrConfigInvalid` at `Run`. |

```go
func WithDefaultRequestTimeout(d time.Duration) Option
func WithDefaultRequestManyWindow(d time.Duration) Option
```

**Validation at Run** (in `bootRuntime`):

- `defaultRequestTimeout <= 0` → `*ErrConfigInvalid{Field: "default_request_timeout", Reason: "non-positive"}`.
- `defaultRequestManyWindow <= 0` → `*ErrConfigInvalid{Field: "default_request_many_window", Reason: "non-positive"}`.

---

## Error types *(public, typed sentinels)*

All errors are exported, satisfy `errors.Is` and `errors.As`, and name
the offending value in `Error()`.

| Error | Trigger | Wrapping |
|---|---|---|
| `*ErrNoResponders` | substrate reports `nats.ErrNoResponders` for a `Request` or a `RequestMany` publish | — (leaf sentinel) |
| `*ErrRequestTimeout` | `Request`'s effective deadline elapsed | `Unwrap() = context.DeadlineExceeded` so `errors.Is(err, context.DeadlineExceeded)` matches |

```go
type ErrNoResponders struct{ Subject string }
type ErrRequestTimeout struct{ Subject string; Timeout time.Duration }

func (e *ErrNoResponders)   Error() string  { return "harness: no responders for subject "+strconv.Quote(e.Subject) }
func (e *ErrRequestTimeout) Error() string  { return "harness: request timeout: subject "+strconv.Quote(e.Subject)+" exceeded "+e.Timeout.String() }
func (e *ErrRequestTimeout) Unwrap() error  { return context.DeadlineExceeded }
```

**Re-exports**: `*ErrConfigInvalid` (from `001-harness-core`) is the
error type returned for non-positive `WithDefaultRequestTimeout` or
`WithDefaultRequestManyWindow` values (FR-119). No new error type is
added for config invariants.

**`RequestMany` does not produce `*ErrRequestTimeout`**: the
window-elapsed condition is a successful return with whatever replies
were collected (FR-110). An empty slice is the legitimate "nobody home"
outcome.

`*ErrNoResponders` *can* still appear from `RequestMany` if the
substrate refuses the publish entirely (e.g., the connection's account
permissions reject the subject before any responder is consulted).
That's a substrate-level failure, not a fan-out outcome.

---

## Metrics *(public, snapshot struct, modified)*

Extended with four new counters.

| Field | Type | Source | Notes |
|---|---|---|---|
| (existing fields) | (as in 001-harness-core) | (unchanged) | |
| `RequestCalls` | uint64 | counter | **NEW**. Total `Request` invocations (success + failure). |
| `RequestManyCalls` | uint64 | counter | **NEW**. Total `RequestMany` invocations. |
| `RequestNoResponders` | uint64 | counter | **NEW**. Invocations of either kind that returned `*ErrNoResponders`. |
| `RequestTimeouts` | uint64 | counter | **NEW**. `Request` invocations that returned `*ErrRequestTimeout`. |

`Metrics.snapshot()` is extended to read these fields.

The earlier draft included a `CapabilityProtocolErrors` counter for
JSON-decode failures; that is gone with the codec layer (R-112).

---

## runtime *(internal, unchanged)*

The unexported `runtime` struct in `harness/imp.go` gains no new
fields. The dispatch helpers in `harness/request.go` are stateless;
they take their inputs (the connection, the metrics, the logger, the
defaults) as arguments. No "surface" struct, no resolved endpoint map.

---

## Awareness/Reasoning context concrete types *(internal, modified)*

`awarenessCtx` and `reasoningCtx` gain methods that delegate to the
shared dispatch helpers:

```go
func (a *awarenessCtx) Request(ctx, subject, payload, opts...) ([]byte, error) {
    return requestSingle(ctx, a.conn, a.metrics, a.logger, a.defaultRequestTimeout, subject, payload, opts)
}

func (r *reasoningCtx) Request(ctx, subject, payload, opts...) ([]byte, error) {
    return requestSingle(ctx, r.conn, r.metrics, r.logger, r.defaultRequestTimeout, subject, payload, opts)
}

func (r *reasoningCtx) RequestMany(ctx, subject, payload, opts...) ([][]byte, error) {
    return requestMany(ctx, r.conn, r.metrics, r.logger, r.defaultRequestManyWindow, subject, payload, opts)
}
```

Both concrete types gain `conn *nats.Conn`, `defaultRequestTimeout
time.Duration`, and (for reasoning only) `defaultRequestManyWindow
time.Duration` fields — populated in `bootRuntime`.

---

## Dispatch helpers *(internal, harness/request.go)*

Two package-level (unexported) functions encapsulate the dispatch:

```go
func requestSingle(
    ctx context.Context,
    nc *nats.Conn,
    m *metrics,
    log logger,
    defaultTimeout time.Duration,
    subject string,
    payload []byte,
    opts []RequestOption,
) ([]byte, error)

func requestMany(
    ctx context.Context,
    nc *nats.Conn,
    m *metrics,
    log logger,
    defaultWindow time.Duration,
    subject string,
    payload []byte,
    opts []RequestManyOption,
) ([][]byte, error)
```

Each:

1. Increments the call-count metric (`RequestCalls` or
   `RequestManyCalls`) before doing any work, so a failure path
   still records the call.
2. Computes the effective timeout / window (per-call > harness default).
3. For `requestSingle`: derives a context via
   `context.WithTimeout(ctx, effective)` and calls
   `nc.RequestWithContext(derivedCtx, subject, payload)` — literal
   subject, no transformation.
4. For `requestMany`: creates an inbox, subscribes a buffered channel
   sized `max(k, 64)`, publishes the request on the literal subject,
   collects replies in a `select` over (replyCh, deadline, ctx.Done()),
   and unsubscribes the inbox in a `defer` that runs regardless of how
   the function returns (FR-113).
5. Translates errors per R-103 / R-104 (no-responders, timeout,
   cancellation, substrate-other).
6. Increments failure-mode counters on the matching error path.

---

## Lifecycle

| Phase | Behavior |
|---|---|
| At `NewImp` | Validation of `ImpSpec` is unchanged from `001-harness-core`. No capability-side validation runs. |
| At `Run` (in `bootRuntime`) | Validate `defaultRequestTimeout > 0` and `defaultRequestManyWindow > 0` (FR-116). Populate the concrete contexts with `nc`, defaults, and the metrics / logger handles. |
| During dispatch | Awareness's `Request` and reasoning's `Request` / `RequestMany` are issued synchronously when the imp's code invokes them. Each call's lifetime is the call duration; nothing outlasts the call. |
| At shutdown | In-flight `Request` and `RequestMany` calls return when the caller's `ctx` is cancelled (the harness shutdown context is what reasoning receives; `001-harness-core` FR-126 analog). The inbox subscription for `RequestMany` is cleaned up on the cancellation path same as on the window-elapse path. |

No new lifecycle state is introduced. The `internal/lifecycle.Machine`
states (`Created` → `Starting` → `Running` → `Draining` → `Stopped`)
are unchanged.

---

## Relationships

```
AwarenessContext  ──exposes──▶ Request
ReasoningContext  ──exposes──▶ Request, RequestMany, Publish, Conn()

Request, RequestMany, Publish ──send on──▶ declared subject verbatim
                              ──share──▶ *nats.Conn (caller-supplied)
                              ──share──▶ metrics (counters incremented per call)

Request, RequestMany ──read──▶ runtimeOptions.defaultRequestTimeout / defaultRequestManyWindow
                     ──may receive──▶ per-call RequestOption / RequestManyOption overrides

Errors:
  *ErrNoResponders     ◀── nats.ErrNoResponders translation
  *ErrRequestTimeout   ◀── derived-context DeadlineExceeded translation
  *ErrConfigInvalid    ◀── non-positive default at bootRuntime
```
