# Soulstream Participation

How an imp takes part in soulstream topics: receiving a topic as a channel,
contributing notes from awareness and turns from thinking, and leaving. This
document is the M1 design (roadmap, "Soulstream coordination channels"),
graduated from the `soulstream-participation` research topic
([episode 0003](../04-JOURNEY/0003-soulstream-participation.md)). Everything
here is **[V]** — shipped as feature `004-soulstream-participation`
([episode 0004](../04-JOURNEY/0004-soulstream-participation-shipped.md)); this
document describes the module as built and tested.

The load-bearing finding this design rests on `[measured]`: the soulstream
protocol has **no join, no leave, and no membership state**. Presence is a
consumer; posting rights are NATS subject permissions. Participation therefore
requires **zero harness changes** — the spike ran the full loop below against
the shipped harness, byte-identical, three `-race` runs green.

## Where it lives: a glue module, not the harness

Participation ships as a **nested Go module** in this repository:

- Directory `soulstream/`, module `github.com/impire-io/imps/soulstream`, its
  own `go.mod`.
- It requires the core module `github.com/impire-io/imps` and the owner's
  client library `github.com/impire-io/soulstream` (measured additional
  transitive cost: `google/uuid`, `gowebpki/jcs`,
  `synadia-io/orbit.go/natscontext`; the soulstream MCP SDK is **not** pulled
  by `realm`/`topic` consumers).
- The core module's `go.mod` MUST NOT change. The harness stays two direct
  dependencies; imps that never touch soulstream inherit nothing.
- `make fmt` / `make test` / `make lint` MUST cover the nested module (the
  Makefile and CI gain the second module path).

The wire contract is **owned by the soulstream repo** (normative:
`hq/02-DESIGN/core/01-protocol.md` and `core/03-topics.md` there; wire version
`1`, vocabulary evolution additive). This module consumes it through the
owner's `realm` and `topic` packages for every write; the read path decodes
three headers and needs no library at all.

## The consumed contract (pinned 2026-07-25, episode 0003)

| Operation | Subject / mechanism |
|---|---|
| "Join" = read | JetStream consumer on stream `SOULSTREAM`, `FilterSubject: SOULSTREAM.TOPICS.OPS.<topic-path>`, deliver-all: baseline first, history then live, one continuous consumer |
| "Leave" | Stop the consumer. No protocol op exists or is needed |
| Post a turn | Op `turn.post` on `OPS.<path>` via `topic.Handle.PostTurn`; `@mention`s fan out `mention.notify` to `SOULSTREAM.PERSONA.NOTIFY.<persona>` |
| Note | Op `comment.add` on `OPS.<path>` anchored to an existing op-id via `topic.Handle.AddComment` |
| Start a topic | `topic.StartTopic`: `topic.announce` on `INFO.<path>` + initial `baseline` as first `OPS` message |
| Close a topic | Op `life.transition{to:"closed"}`; advisory, not enforced |
| Inbox | Stream `SOULSTREAM_NOTIFY`, subject `SOULSTREAM.PERSONA.NOTIFY.<persona>`; bounded window, no cursor |
| Discovery | Request-reply on `SOULSTREAM.SVC.DISCOVER`; asker-enforced deadline |

Wire obligations when writing (all handled by the owner's library): headers
`Nats-Msg-Id` (writer UUID, held across retries; 2-minute dedup window makes
publish idempotent), `Soulstream-Version: 1`, `Soulstream-Author` (must equal
the client's persona), `Soulstream-Parents` (observed frontier; empty is legal
and merge-safe), `Soulstream-Ts` (informational), optional `Soulstream-Sig`
(Ed25519 over the RFC-8785 canonical record). Realm scoping is the NATS
account boundary, not a subject token: an imp participates in the realm its
connection reaches.

## Surfaces

### `TopicChannel` — a topic as a channel

```go
func TopicChannel(path string, opts ...TopicChannelOption) imps.ChannelSpec
```

Returns a `ChannelSpec` whose `Source` is the **existing** `StreamSource`:
stream `SOULSTREAM`, `FilterSubject` the topic's OPS subject,
`DeliverPolicy: DeliverAllPolicy` by default. The default `Decode` yields an
`Op{Type, Author, ID string}` from the `Soulstream-Type`, `Soulstream-Author`,
and `Nats-Msg-Id` headers — no payload parsing on the dispatch path. The
default `ExtractEntity` returns the topic path as the entity.

- Options MAY override the decoder (e.g. payload-parsing), the entity
  extractor, and the consumer configuration passthrough the harness already
  exposes (`Durable` for catch-up across restarts, `OptStartSeq`/`OptStartTime`
  for warm rejoin).
- The channel MUST NOT materialise the topic on the dispatch path. Ops arrive
  raw, in stream order, baseline first; awareness interprets cheaply.
  Materialised views are a thinking-tier concern.
- Joining a topic is declaring this channel in `ImpSpec.Channels`. Leaving is
  imp shutdown (the harness already deletes ephemeral consumers). This is
  **static participation**: the topic set is fixed at `Run`. Runtime
  join/leave is out of scope (see Decisions).

### `NoteBridge` — awareness's `Note` becomes a contribution

```go
type Noted struct {
    AnchorOp string // op-id the note annotates (required)
    Body     string
}

func NoteBridge(p *Participant, next func(imps.Entity, any), onErr func(imps.Entity, Noted, error)) func(imps.Entity, any)
```

An `OnNote` implementation. When the payload is a `Noted`, it posts
`comment.add` on the entity's topic (the entity **is** the topic path under
`TopicChannel`'s default extractor), anchored to `AnchorOp`, authored as the
participant's persona. Any other payload type is passed to `next` (nil `next`
drops it), so soulstream notes compose with local note handling. A `Noted`
with an empty anchor or body, an empty entity, or a publish failure goes to
`onErr` (nil drops); nothing malformed is ever published, and the publish is
bounded by an internal timeout so a dead substrate cannot hang dispatch.

- Awareness's surface is **unchanged** — it returns the shipped `Note`
  verdict; the bridge does the publishing outside awareness's hands. The
  compile-deny invariants are untouched. `[measured]` in the spike.
- The bridge posts with a best-effort (empty) frontier — legal and merge-safe;
  the anchor op-id is the op awareness just observed, so no materialise call
  and no frontier state exist anywhere in the note path. `[measured]`
- The bridge runs synchronously on the dispatch goroutine, so a note costs one
  JetStream publish round-trip on that goroutine. Accepted: notes are
  low-rate by definition (the energy gradient's cheap tier). If a real imp
  shows note-rate pressure, the bridge gains an internal queue — a glue-module
  change, not a harness change.

### `Participant` — identity and the write path

```go
func NewParticipant(ctx context.Context, nc *nats.Conn, realm, persona string, opts ...ParticipantOption) (*Participant, error)
```

Wraps the imp's **own** NATS connection in the owner's `realm.NewClient`
(`[measured]`: no second connection needed). Persona is required for any write
path; an optional Ed25519 signer (`WithSigner`) makes every op signed. The
Participant MUST NOT close the wrapped connection (the imp owns it) — and
because the owner's client closes a connection it fails to construct around,
`NewParticipant` confirms JetStream reachability itself first, so a
construction failure leaves the connection open `[measured]` (regression test
in `participant_test.go`).

Thinking-tier operations use the owner's library through the Participant:
`StartTopic`, `Open(path)` → `PostTurn` / `AddComment` / `Close`, materialise
on demand. The glue module re-exports nothing it doesn't have to — thinking
holds a `topic.Handle` from the owner's package directly. Discovery
(`SVC.DISCOVER`) remains a plain `Request` from either tier; answering
discovery is not an imp concern.

## Boundaries (MUST / MUST NOT)

- The harness core MUST NOT change: no new channel kind, no dispatch change,
  no `AwarenessContext` change, no new core dependency. (The spike proved the
  loop needs none of them.)
- Awareness MUST NOT open, close, or leave topics; its only soulstream
  emission is the `Note` verdict through the bridge. Topic lifecycle
  (`StartTopic`, `Close`) is thinking-tier.
- The glue module MUST NOT model membership, ordering beyond stream order, or
  rollup/archival duties (operator concerns; a plain participant has no rollup
  obligation per the owner's spec).
- Attribution: one imp = one persona per Participant; the owner's library
  already refuses cross-persona authorship.

## Acceptance criteria

1. An imp declaring `TopicChannel(path)` receives, through the unmodified
   dispatch seam, the topic's baseline first, then history, then live ops, in
   stream order, over one continuous consumer — verified against a soulstream
   realm provisioned by the owner's `ProvisionOn`.
2. An awareness `Note(Noted{...})` verdict on an observed op results in a
   `comment.add` on the topic, anchored to that op, authored as the imp's
   persona, visible as a first-class non-dangling comment in the owner
   library's materialised view.
3. A turn posted from thinking via the Participant is received by another
   participant's channel and materialises with correct attribution.
4. Imp shutdown deletes the channel's ephemeral consumer; a durable-configured
   channel resumes from its cursor on restart.
5. The core module's `go.mod` is byte-identical before and after the feature;
   `make compile-deny` stays green; the full gate covers the nested module.
6. Unknown op types dispatch like any other op (decode yields the type;
   awareness decides) — additive vocabulary evolution needs no glue release.

## Decisions and tradeoffs

- **Glue module over harness feature or wire reimplementation.** The
  adversarial pass (episode 0003) refuted both alternatives: reimplementing
  the write path duplicates owner-maintained canonicalisation/signing that
  drifts silently; importing the owner's library into the core taxes every
  imp with coupling to a pre-1.0 module. The glue module keeps the harness
  small (constitutional) while the owner's code does the writes.
- **Static participation, deliberately.** The protocol needs no runtime
  join/leave, and no current use case demands one. Reversal condition
  `[judgment]`: the first real colony scenario in which an imp's topic set
  must change without a restart reopens runtime channel lifecycle (the
  per-channel bind/teardown structure in `start.go`/`stream.go` is already
  shaped for it).
- **Raw ops on the dispatch path, materialisation in thinking.** Keeps
  awareness cheap (three header reads), keeps CRDT merge machinery out of the
  hot path, and matches "channels deliver messages; the framework does not
  interpret."
- **Synchronous note bridge.** Simplest shape that works; measured harmless at
  spike rates; upgrade path (internal queue) named above and confined to the
  glue module.
- **Inbox and mention channels** (`SOULSTREAM_NOTIFY`) are a natural follow-on
  `TopicChannel` sibling but are not required by M1's exit criteria and are
  not specified here.
