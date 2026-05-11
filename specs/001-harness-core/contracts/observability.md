# Contract: Observability

What the harness exposes for monitoring and debugging — and what it does *not* expose. The boundary matters: this is the harness's full observability surface for v1, and integrations with external metrics, tracing, or audit systems are deferred features.

---

## In-flight reasoning count (FR-021b)

A gauge tracking the number of currently-running reasoning invocations on the imp.

**Reads**:
- `ReasoningContext.InFlight() int` — readable from inside reasoning code itself (a reasoning fn that wants to self-throttle can read this).
- `Imp.Metrics().InflightReasoning` — readable by external code holding the imp handle.

**Semantics**:
- Incremented immediately before the reasoning goroutine starts.
- Decremented when the reasoning function returns (normally or with error) or panics.
- Atomic; both reads return a consistent point-in-time value but may not agree across reads — there is no global lock.

This satisfies the spec's clarification Q3: "The harness MUST expose the current in-flight reasoning count via its observability surface so developers can monitor and react."

---

## Metrics snapshot

`Imp.Metrics()` returns a non-resetting snapshot of all counters and the in-flight gauge. Counters are monotonic across the lifetime of the imp.

| Counter | Description | Increments at |
|---|---|---|
| `InflightReasoning` (gauge, int64) | currently-running reasoning | reasoning start (+1) / end (-1) |
| `DecodeFailures` | decode returned error | per message (FR-006) |
| `ExtractionFailures` | extractor errored or returned empty entity | per message (FR-007) |
| `AwarenessPanics` | awareness panicked | per message (FR-015) |
| `ReasoningPanics` | reasoning panicked | per reasoning invocation (FR-021) |
| `ReasoningErrors` | reasoning returned non-nil error | per reasoning invocation (FR-021) |
| `NotesDelivered` | Note verdict | per dispatch returning Note (FR-012) |
| `WakesDispatched` | Wake verdict (whether or not reasoning has yet started) | per dispatch returning Wake (FR-013) |
| `IgnoredVerdicts` | Ignore verdict | per dispatch returning Ignore (FR-011) |
| `NakTotal` | stream-channel NAK | per NAK (FR-008a) |

**Thread safety**: `Metrics()` is safe to call concurrently with dispatch and reasoning. Counter reads are atomic; the snapshot is not transactionally consistent across counters (a counter incremented during the snapshot may or may not be visible).

---

## Note hook (FR-012)

```go
ImpSpec.OnNote func(entity Entity, payload any)
```

If non-nil, called synchronously per `Note` verdict before dispatch returns. If nil, the verdict is recorded in `NotesDelivered` but the payload is dropped.

**Constraints**:
- The hook MUST be fast — it runs inside dispatch (FR-009). Long-running work in the hook backpressures awareness.
- The hook MUST NOT panic. If it does, the harness recovers and increments `AwarenessPanics` (treating the panic as having occurred during the dispatch path). The originating message is still acked per FR-008a because the verdict was returned.

This is the v1 surface; soulstream emission of Note records is the soulstream feature's job.

---

## Logger

`WithLogger(slog.Handler)` configures the harness's structured logger. Default is a discard handler (no output).

**Events the harness logs** (level: INFO unless noted):
- imp start: `name`, `version`, channel count
- imp ready: after all subscriptions established
- imp shutdown begin: with drain window
- imp shutdown end: with timing and pending-reasoning count at deadline (WARN if non-zero)
- subscription failure: subject, cause (ERROR)
- decode failure: channel, subject, entity-stage skipped (WARN)
- extraction failure: channel, decoded type (WARN)
- awareness panic: channel, entity, recovered stack (ERROR)
- reasoning panic: entity, reason summary, recovered stack (ERROR)
- reasoning error: entity, error (WARN)
- stream channel: durable bound, ephemeral created, ephemeral deleted (INFO)

The harness does NOT log message bodies.

---

## What the harness does NOT expose in v1

These are deliberate omissions, deferred to follow-up features:

- **No Prometheus / OpenTelemetry metrics export.** The `Metrics()` struct is the v1 surface; an OTEL adapter is a separate package.
- **No tracing.** Span propagation through awareness/reasoning context is not implemented in v1.
- **No per-channel metric breakdowns.** Counters are imp-wide.
- **No audit-record emission.** Audit-stream integration is out of scope (FR-NS-2).
- **No soulstream emission of Note records.** (FR-NS-2.)
- **No detailed event log of dispatched messages.** Counters and structured logger events are the surface.
- **No GC-pressure / goroutine-count introspection.** Standard `runtime/pprof` is the right answer for those.

A future observability feature can add any of the above without changing the v1 contract — `Metrics()` and `OnNote` and the slog handler all remain stable.

---

## Probing identity (FR-003)

```go
i.Identity()  // returns ImpIdentity{Name, Version}
i.Ready()     // bool: true once startup has registered subscriptions
```

`Identity()` is available in every lifecycle state; the values are derived from the spec at construction time. `Ready()` flips from `false` to `true` when the lifecycle reaches `Running`.

`Identity()` is NOT registered with NATS in v1 — it is in-process only. Discovery via `$SRV.INFO` belongs to a future capability/discovery feature.
