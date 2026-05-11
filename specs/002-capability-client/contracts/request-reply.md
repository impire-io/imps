# Contract: Request/Reply Surface

The harness's developer-facing outbound surface. This document specifies
the API that imp authors call from awareness and reasoning, what each
method guarantees, the typed errors that result, and how the calls
interact with `001-harness-core`'s existing lifecycle machinery.

Package: `github.com/impire-io/imps/harness`

---

## Spec extension

No changes to `ImpSpec`. This feature adds methods to the context
interfaces and two harness-construction options.

---

## Construction options

Two new options on `harness.Options`, applied at `NewImp`, validated at
`Run`:

```go
func WithDefaultRequestTimeout(d time.Duration) Option       // default 5 s
func WithDefaultRequestManyWindow(d time.Duration) Option    // default 1 s
```

Defaults:

- `WithDefaultRequestTimeout` → `5 * time.Second` (FR-116).
- `WithDefaultRequestManyWindow` → `1 * time.Second` (FR-116).

Startup config validation:

- `defaultRequestTimeout <= 0` → `*ErrConfigInvalid{Field: "default_request_timeout", Reason: "non-positive"}`.
- `defaultRequestManyWindow <= 0` → `*ErrConfigInvalid{Field: "default_request_many_window", Reason: "non-positive"}`.

Both errors satisfy the existing `001-harness-core` `*ErrConfigInvalid`
shape (FR-119).

---

## Context interfaces

Methods added to the existing context types defined in
`001-harness-core/contracts/public-api.md`.

### AwarenessContext

```go
type AwarenessContext interface {
    State(name string, entity Entity) (StateRef, error)

    // Request issues a single NATS request/reply on the declared subject
    // verbatim — the framework performs no transformation (constitution
    // v2.2.0 "Imps see one subject path").
    //
    // The effective timeout is the per-call WithRequestTimeout if positive,
    // otherwise the harness's WithDefaultRequestTimeout. The call honors
    // the caller's ctx as both a cancellation source and an upper bound.
    //
    // Returns the reply payload bytes on success, or one of:
    //   *ErrNoResponders     — substrate reports no handler
    //   *ErrRequestTimeout   — deadline elapsed
    //   context.Canceled-wrapping — caller's ctx was cancelled
    //   substrate-specific (e.g., nats.ErrMaxPayload) — passed through.
    //
    // No retry is attempted; the call is one-shot.
    Request(
        ctx context.Context,
        subject string,
        payload []byte,
        opts ...RequestOption,
    ) ([]byte, error)

    // (No RequestMany method. No Publish method. No Conn method. Their
    // absence is the compile-time enforcement of the energy gradient —
    // awareness is bounded by call shape: single-round-trip
    // request/reply only.)
}
```

### ReasoningContext

```go
type ReasoningContext interface {
    State(name string, entity Entity) (StateRef, error)
    InFlight() int

    // Publish (unchanged from 001-harness-core v2.2.0 cleanup).
    // Publishes the payload on the declared subject verbatim. No
    // framework-side whitelist; substrate ACLs gate.
    Publish(ctx context.Context, subject string, payload []byte) error

    // Request — same semantics as AwarenessContext.Request.
    Request(
        ctx context.Context,
        subject string,
        payload []byte,
        opts ...RequestOption,
    ) ([]byte, error)

    // RequestMany issues a single NATS request on the declared subject
    // verbatim and collects every reply received within the effective
    // window (per-call WithRequestManyWindow if positive, otherwise
    // WithDefaultRequestManyWindow). An optional per-call cap
    // (WithRequestManyMax(k)) causes the call to return after k replies
    // have arrived without waiting for the rest of the window.
    //
    // Returns the collected reply payloads as a slice (possibly empty
    // when no responders replied within the window — that is a
    // legitimate "nobody home" outcome, not an error).
    //
    // On caller-ctx cancellation, returns a partial slice (or nil) and
    // an error wrapping context.Canceled.
    //
    // On substrate publish failure (e.g., nats.ErrNoResponders if the
    // substrate refuses the publish before any responder can reply),
    // returns nil and the substrate error wrapped as *ErrNoResponders.
    //
    // The temporary inbox subscription used to collect replies is
    // unsubscribed on every return path. No retry.
    RequestMany(
        ctx context.Context,
        subject string,
        payload []byte,
        opts ...RequestManyOption,
    ) ([][]byte, error)

    // Conn returns the raw NATS connection. Escape hatch for generic
    // NATS-based clients used from reasoning (an inference client,
    // a knowledge client, etc.) that take *nats.Conn directly. Not
    // available on awareness.
    Conn() *nats.Conn
}
```

The `ctx` cancellation continues to be wired to harness shutdown
(`001-harness-core`'s reasoning context cancellation pattern). In-flight
`Request` / `RequestMany` calls MUST return promptly with a
`context.Canceled`-wrapping error when shutdown begins.

---

## Compile-time guarantees (verified by build)

The following must hold at compile time:

1. **`AwarenessContext` has no `RequestMany` method.** A build-tagged
   file under `integration/compiletest/awareness_no_requestmany.go`
   under tag `awareness_requestmany_must_fail` asserts this (SC-104).
2. **`AwarenessContext` has no `Publish` method** (existing 001 guarantee).
3. **`AwarenessContext` has no `Conn` method.** Added in the same
   feature; a build-tagged file under
   `integration/compiletest/awareness_no_conn.go` under tag
   `awareness_conn_must_fail` asserts this.

CI runs `go vet -tags=<tag> ./integration/compiletest/...` for each
tag and gates on a non-zero exit (the build failure is the assertion).

---

## Per-call options

```go
type RequestOption     func(*requestOptions)
type RequestManyOption func(*requestManyOptions)

func WithRequestTimeout(d time.Duration) RequestOption
func WithRequestManyWindow(d time.Duration) RequestManyOption
func WithRequestManyMax(n int)            RequestManyOption
```

| Option | Semantics |
|---|---|
| `WithRequestTimeout(d)` | When `d > 0`, overrides the harness's `defaultRequestTimeout` for this call. When `d <= 0`, silent no-op. |
| `WithRequestManyWindow(d)` | When `d > 0`, overrides the harness's `defaultRequestManyWindow` for this call. When `d <= 0`, silent no-op. |
| `WithRequestManyMax(n)` | When `n > 0`, the call returns as soon as `n` replies arrive. When `n <= 0`, the call collects for the full window. |

---

## Effective timeout / window

For `Request`:

- effective = `WithRequestTimeout(d)` if `d > 0`, else `defaultRequestTimeout`.
- The harness derives `context.WithTimeout(ctx, effective)` and calls
  `nc.RequestWithContext` on the literal declared subject. The earlier
  of (caller ctx, derived deadline) fires first.

For `RequestMany`:

- effective window = `WithRequestManyWindow(d)` if `d > 0`, else
  `defaultRequestManyWindow`.
- effective cap = `WithRequestManyMax(n)` if `n > 0`, else "no cap".
- The harness publishes on the literal declared subject, then collects
  in a `select` over (replyCh, `time.After(effective)`, `ctx.Done()`);
  the cap is checked after each received reply.

---

## Subjects

The declared subject is the wire subject — verbatim. The framework
imposes no transformation. Per the constitution's "Imps see one subject
path" principle (v2.2.0), cross-account routing is configured at the
substrate via NATS account imports, not encoded in framework code.

The subject appears in the `Subject` field of `*ErrNoResponders` and
`*ErrRequestTimeout`.

---

## Whitelist / subject permissioning

There is no framework-side whitelist. `Publish`, `Request`, and
`RequestMany` all delegate gate enforcement to the substrate. NATS
account ACLs on the imp's connection are the authoritative mechanism —
a publish (or request) the ACLs forbid is rejected by the substrate
and the harness returns the substrate's error verbatim.

This matches the 001-harness-core v2.2.0 cleanup that removed
`ImpSpec.Actions` and `*ErrWhitelistViolation`.

---

## Errors

Two new exported sentinel types, satisfying `errors.Is` / `errors.As`.
`Error()` messages include the offending value(s).

```go
type ErrNoResponders   struct{ Subject string }
type ErrRequestTimeout struct{ Subject string; Timeout time.Duration }
```

| Error | Trigger | Wrapping |
|---|---|---|
| `*ErrNoResponders` | substrate reports `nats.ErrNoResponders` for either `Request` or `RequestMany` (the latter only when the substrate refuses the publish entirely) | — |
| `*ErrRequestTimeout` | `Request`'s effective deadline elapsed before a reply arrived | `Unwrap() = context.DeadlineExceeded` so `errors.Is(err, context.DeadlineExceeded)` matches |

**Error category invariants**:

- `Request` produces exactly one of: success, `*ErrNoResponders`,
  `*ErrRequestTimeout`, `context.Canceled`-wrapping, or a passed-through
  substrate error.
- `RequestMany` produces exactly one of: success (possibly empty slice),
  `context.Canceled`-wrapping (with partial slice or nil), or
  `*ErrNoResponders`/substrate-passthrough on publish refusal.
- Application-level errors (a responder signaling "no" via headers or
  payload) are *not* a framework category. The framework returns
  success; the imp's code interprets the reply.

---

## Observability

Four new counters on `Metrics`:

```go
type Metrics struct {
    // ... existing fields (001-harness-core)
    RequestCalls         uint64
    RequestManyCalls     uint64
    RequestNoResponders  uint64
    RequestTimeouts      uint64
}
```

Calls through `r.Conn()` bypass these counters. The framework-method
path is the observable surface; raw-connection access is intentionally
not.

Log lines emitted by the request-reply subsystem:

- DEBUG (optional): `"request"` per `Request` invocation with
  `subject`, `bytes`, `elapsed`, `outcome`.
- DEBUG (optional): `"request_many"` per `RequestMany` with `subject`,
  `replies`, `elapsed`.
- WARN: `"request failed"` per `Request` failure with `subject`,
  `category`, `cause`.

The harness does not emit a per-call audit record in v1; audit is its
own feature.

---

## What this contract does NOT cover

- **The wire protocol of any responder.** Each responder defines its
  own request/reply shape (FR-NS-105). The framework consumes bytes.
- **JSON marshaling, proto, or any other codec.** Imps that want typed
  payloads bring their own (FR-NS-104).
- **Application-level error signaling.** Responders that signal "I
  refuse" via headers or payload structure use their own protocol; from
  the framework's perspective, that reply is a successful `Request`
  return.
- **Streaming responses.** `RequestMany` is collect-with-window;
  multi-reply streams terminated by a sentinel are a future enhancement
  (FR-NS-106).
- **Per-call request headers.** Not exposed in v1; the byte-level
  surface is symmetric with `Publish` (FR-NS-108).
- **Queue-group or sharding policy.** Multiple responders form queue
  groups at the substrate layer; the client surface is unaware
  (FR-NS-107).
- **Retry, backoff, circuit-breaker.** None. The framework MUST NOT
  publish a second request on the same subject in response to a failure
  (FR-109, FR-114, SC-106).

---

## Symmetry with `001-harness-core`

The request/reply surface mirrors the existing harness contracts:

- Subjects pass through verbatim (no resolver, no prefix — constitution
  v2.2.0).
- Errors are typed sentinels in the same style
  (`Field`/`Subject`/`Cause` carry the offending value).
- The energy gradient is enforced the same way: by *not* exposing a
  method on awareness that allows unbounded operation.
- No retry / backoff / fallback (FR-NS-103 mirrors `001-harness-core`
  FR-NS-3).
- Byte-shaped payloads, no framework-imposed codec (symmetric with
  `Publish`'s byte payload).

A reader who knows `001-harness-core` can read the request/reply
surface without learning new ceremony.
