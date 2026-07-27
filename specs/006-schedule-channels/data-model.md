# Data Model: Schedule Channels

The package holds **no state** — no registry, no timers, no cursors. Its
types are views and builders over substrate-owned data.

## Tick — one firing, as the imp sees it

| Field | Source header | Rules |
|---|---|---|
| `Subject` | (message subject) | The target subject the tick arrived on; the default entity. |
| `Scheduler` | `Nats-Scheduler` | The schedule subject that produced the tick — provenance, present on every server-emitted tick. |
| `Next` | `Nats-Schedule-Next` | Raw marker: RFC3339 of the next firing, or the server's purge marker on a final firing. Informational. |

- Produced by the default header-only decoder; payload untouched (override
  for payload-bearing ticks).
- Never filtered: unknown headers/payloads flow to awareness.

## Schedule (substrate-owned; the package only writes it)

One message per schedule subject in an `AllowMsgSchedules` stream:

| Option | Header written | Validation (client-side only) |
|---|---|---|
| pattern (required) | `Nats-Schedule` | non-empty; semantics are the server's |
| target (required) | `Nats-Schedule-Target` | non-empty |
| `WithTickTTL(d)` | `Nats-Schedule-TTL: <d>` | `d > 0`; formatted as a Go duration; documented as requiring `AllowMsgTTL` |
| `WithTimeZone(tz)` | `Nats-Schedule-Time-Zone` | non-empty when given (cron patterns only, per server) |
| `WithBody(b)` | (message payload) | — |
| `WithSource(subj)` | `Nats-Schedule-Source` | non-empty when given |
| `WithRollup()` | `Nats-Schedule-Rollup` | — |

- Re-register on the same subject ⇒ replaced (server keys schedules by
  subject).
- `Deregister` ⇒ purge the schedule subject; ticks already emitted are
  unaffected.
- Absence of `WithTickTTL` is legal and means full accumulation — the
  consequence is documented at the option.

## Channel configuration

Identical passthrough discipline to `soulstream.TopicChannel`: durable name,
start sequence/time, decoder, entity extractor, display name
(default `"schedule:"+target`); source is always the existing
`imps.StreamSource` (stream + target as filter subject, deliver-all
default).

## Relationships

```text
Register(js, schedSubj, pattern, target, opts) ──one publish──▶ stream (schedule message)
                                                                    │ server clock
                                                                    ▼
                                                       tick on target subject (+provenance)
                                                                    │
ImpSpec.Channels ── Channel(stream, target) ── StreamSource ──▶ dispatch ──▶ awareness(Tick)
Deregister(js, stream, schedSubj) ──purge──▶ schedule gone; emitted ticks untouched
```
