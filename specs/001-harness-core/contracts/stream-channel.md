# Contract: Stream Channels

JetStream-backed channel behavior. Honors FR-004b, FR-005, FR-005a, FR-005b, FR-005c, FR-008a; User Story 4 (P1) and its acceptance scenarios.

---

## Declaration

A stream channel is declared via `ChannelSpec.Source = StreamSource{...}`:

```go
type StreamSource struct {
    Stream         string         // JetStream stream name (required)
    FilterSubject  string         // pre-resolution subject or pattern within the stream (required)
    Durable        string         // empty = ephemeral consumer; non-empty = bind/create durable
    ConsumerConfig ConsumerConfig // passthrough fields for the substrate
}
```

The harness applies subject resolution to `FilterSubject` (see `subject-resolution.md`) before building the consumer's filter.

`ConsumerConfig` exposes the consumer-config fields the harness supports as passthrough to JetStream:

| Field | Default if zero | Notes |
|---|---|---|
| `AckPolicy` | `AckExplicit` | The harness requires explicit ack to enforce FR-008a. |
| `AckWait` | substrate default | Caller may override. |
| `MaxDeliver` | substrate default (-1, unlimited) | Caller may override. |
| `DeliverPolicy` | `DeliverAll` | Caller may override. |
| `ReplayPolicy` | `ReplayInstant` | Caller may override. |
| `OptStartSeq` / `OptStartTime` | unset | Caller may override. |
| `MaxAckPending` | substrate default | Caller may override. |
| `Description` | empty | Operator hint. |

Fields not listed are reserved; the harness will not silently set them. Adding a field requires a spec amendment.

---

## Startup behavior

For each declared stream channel, the harness performs these steps in order at `Run`:

1. **Stream existence check**:
   - Lookup stream `Stream` via `jetstream.Stream(ctx, name)`.
   - If not found → `ErrStreamNotFound{Stream: name}`. Startup aborts; no subscriptions remain (FR-005c, User Story 4 acceptance scenario 3).

2. **Consumer resolution**:
   - **Durable empty** (ephemeral): create a new consumer with the channel's declared `ConsumerConfig` plus a generated name. Track the consumer name in `ChannelState.consumerName` for teardown on shutdown (FR-005b).
   - **Durable non-empty**:
     - Lookup consumer `Durable` on stream `Stream`.
     - If not found: create with declared `ConsumerConfig`. Record consumer name.
     - If found: compare existing consumer's config against declared config (see "Compatibility check" below). On compatible: bind. On incompatible: `ErrConsumerIncompatible{Consumer, Diff}`. Startup aborts (FR-005a, User Story 4 acceptance scenario 4).

3. **Pull/consume start**:
   - The harness uses JetStream's `Consume` API (push-style callback over a pull consumer). Each delivered message goes through the same dispatch path as a subject channel: decode → entity extract → awareness → (optional) reasoning queue.

---

## Compatibility check (durable consumer config)

When a declared durable consumer name `D` already exists on stream `S`, the harness compares:

| Field | Compatibility rule |
|---|---|
| `FilterSubject` | Existing must equal the resolved declared filter subject. |
| `AckPolicy` | Existing must be `AckExplicit` (the harness requires this). |
| `DeliverPolicy` | If declared explicitly, existing must match. If declared zero/default, no check. |
| `ReplayPolicy` | Same as DeliverPolicy. |
| `AckWait` | Existing must be ≥ declared (existing wait covers declared SLA). |
| `MaxDeliver` | If declared, existing must equal. |
| `MaxAckPending` | Informational only — logged on mismatch, not a startup failure. |

Compatibility check is intentionally narrow — it covers only the fields whose drift would change observable behavior. Fields outside the declared `ConsumerConfig` are not compared.

`ErrConsumerIncompatible.Diff` is a human-readable summary, e.g., `filter_subject: existing="orders.*", declared="orders.created"`.

---

## Per-message dispatch and ack timing (FR-008a)

For each delivered message:

```
1. Read msg.Data, msg.Subject, msg.Headers from the JetStream delivery.
2. decoded, err := channel.Decode(message)
   - err != nil       → record DecodeFailure, NAK msg, return.
3. entity, err := channel.ExtractEntity(decoded)
   - err != nil       → record ExtractionFailure, NAK msg, return.
   - entity == ""     → record ExtractionFailure, NAK msg, return.
4. verdict := safeAwareness(decoded, entity, awarenessContext)
   - panic           → record AwarenessPanic, NAK msg, return.
5. ACK msg.   <-- ack happens here, regardless of verdict (FR-008a, clarification Q2)
6. switch verdict.kind {
     case Ignore:  done.
     case Note:    invoke OnNote(entity, payload); done.
     case Wake:    queue reasoning(reason, entity).  // async
   }
```

Reasoning success or failure does NOT change the ack/NAK state of the originating message (FR-008a, User Story 4 acceptance scenario 5).

---

## NAK and redelivery semantics

The harness NAKs in the cases listed above. The substrate (JetStream + consumer's `MaxDeliver`) governs whether the message is redelivered. The harness does not implement its own retry, dead-letter, or backoff (FR-NS-3, edge case "max-deliveries exceeded on the consumer"); a stuck-message signal is observable via the harness's `NakTotal` counter and the substrate's own consumer stats.

If the ack itself fails on the substrate (transient JetStream error during ack), the harness records the failure on the logger and continues — the substrate's own redelivery semantics govern (edge case "ack failure on the substrate").

---

## Shutdown teardown

On clean shutdown:
- For ephemeral consumers (Durable empty): the harness deletes the consumer (FR-005b — "MUST be torn down on clean shutdown" / "no orphaned consumer remains").
- For durable consumers (Durable non-empty): the harness leaves the consumer in place; consumer state survives across imp restarts.

If shutdown is forced (drain window elapsed with reasoning still in flight), the harness still attempts to delete ephemeral consumers — failure to delete is logged but does not block shutdown's return.

---

## Observability

Beyond the global metrics (`Imp.Metrics()`), stream-channel health is visible through:
- The substrate's own `nats consumer info <stream> <consumer>` (operator-side, not harness-API).
- `Metrics.NakTotal` (counter), incremented on every NAK the harness issues.

The harness does not expose per-channel breakdowns of metrics in v1 — the global counters are sufficient for User Story 4's acceptance scenarios.
