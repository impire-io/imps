# Data Model: Soulstream Participation

The module holds **no persistent state of its own** — all durable state lives
on the substrate (owned and provisioned by the soulstream project) or in the
owner library's per-use handles. The entities below are the module's in-memory
types and their rules.

## Op — the imp's view of one topic operation

| Field | Type | Source | Rules |
|---|---|---|---|
| `Type` | `string` | `Soulstream-Type` header | Never filtered by the module; unknown types flow to awareness (additive vocabulary). |
| `Author` | `string` | `Soulstream-Author` header | Persona slug; empty on malformed ops (delivered as-is — awareness judges). |
| `ID` | `string` | `Nats-Msg-Id` header | The op-id; the value a note anchors to. |

- Produced by the default decoder on every dispatched topic message.
- MUST be constructed from headers only — no payload parse, no materialise.
- Relationships: `Op.ID` is the anchor target of `Noted`; `Op.Author` lets
  awareness distinguish self-echo from others' contributions.

## Noted — the note-bridge payload

| Field | Type | Rules |
|---|---|---|
| `AnchorOp` | `string` | REQUIRED — op-id the note annotates (normally the `Op.ID` awareness just observed). Empty → bridge error path, nothing published. |
| `Body` | `string` | REQUIRED — the comment body. Empty → bridge error path, nothing published. |

- Carried by the framework's existing `Note(payload)` verdict; consumed by
  `NoteBridge`.
- Validation happens in the bridge (the harness treats the payload as opaque
  `any` — unchanged).
- State transition: `Noted` → `comment.add` op on the entity's topic, authored
  as the participant's persona, anchored to `AnchorOp`, best-effort frontier.

## Participant — the imp's soulstream identity

| Field | Type | Rules |
|---|---|---|
| wrapped connection | `*nats.Conn` | REQUIRED; the imp's own connection. The Participant MUST NOT close it, ever. |
| realm | `string` | REQUIRED; slug-validated by the owner library; bound into canonical records. |
| persona | `string` | REQUIRED for any write; slug-validated. One persona per Participant. |
| signer | optional Ed25519 key | When present, every posted op is signed (owner library behavior). |

- Constructed once per imp; shared by the note bridge and thinking-tier code.
- Write attribution: author always equals the persona (owner library refuses
  otherwise); the module surfaces, never re-implements, this guard.
- Yields per-topic handles (`Topic(path)`) for thinking-tier operations; the
  handle's frontier is per-use state inside the owner library, not module
  state.

## TopicChannel configuration

| Option | Effect | Rules |
|---|---|---|
| (default) | Ephemeral consumer, deliver-all | Baseline-first full replay then live; consumer deleted on shutdown by the harness. |
| durable name | Durable consumer | Resume-from-cursor across restarts; name uniqueness per imp instance is the operator's concern (documented). |
| start sequence / start time | Warm rejoin | Passthrough of the harness's existing consumer config; mutually exclusive with default deliver-all semantics (the harness's existing validation applies). |
| decoder override | Replaces header-only decode | Same `Decoder` contract as any channel. |
| entity extractor override | Replaces topic-path entity | Same `EntityExtractor` contract; empty entity remains an extraction failure (harness behavior). |

- Output is a plain `imps.ChannelSpec`; the harness owns everything after
  construction (bind, dispatch, ack, teardown).
- Channel name defaults to a stable derivation of the topic path (observable
  in harness logs/metrics).

## Relationships (one picture)

```text
ImpSpec.Channels ──contains──▶ TopicChannel(path) ──produces──▶ ChannelSpec (harness-owned)
                                                                    │ dispatch
                                                                    ▼
                                                    Op{Type, Author, ID} ──▶ Awareness
                                                                                │ Note(Noted{...})
                                                                                ▼
ImpSpec.OnNote = NoteBridge(participant, next) ──publishes──▶ comment.add on OPS.<path>
                                                                    ▲
Thinking ──Participant.Topic(path)──▶ owner library Handle ──turns/comments/close──┘
```
