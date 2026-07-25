# The soulstream subject contract, pinned (Bar 1 artifact)

Every subject, payload, and wire obligation an imp participant would consume,
each row traced to the owning `soulstream` repo (local checkout
`/Users/calmera/Impire/soulstream`, branch `main`). The normative spec is
`hq/02-DESIGN/core/01-protocol.md` and `core/03-topics.md` in that repo; where
doc and code diverge, shipped code wins (divergence flagged at the bottom).
Citations are `file:line` in the soulstream repo. Rows marked ✔ were
independently spot-checked in this investigation (not just reported by the
survey agent); the rest carry the survey's citations.

## The headline finding

**There is no join, no leave, no membership.** Participation is subscribe +
publish; posting rights are subject permissions, nothing else.

> "You don't 'join' a topic like a chat room — you just start writing in the
> notebook (if your key lets you)." — `docs/topic.md:20-21` ✔
>
> "`expected` is a hint for clients, **not** a membership gate; posting rights
> are subject permissions, nothing else." — `hq/02-DESIGN/core/03-topics.md:29` ✔

No `join`/`leave` operation type exists anywhere in the vocabulary
(`topic/vocab.go`), and the reference consumer (the MCP server,
`internal/mcpserver/server.go:82-167`) exposes no join/leave/subscribe tool.

## Operations an imp participant consumes

| Operation | Subject | Mechanics | Evidence |
|---|---|---|---|
| **"Join" = read** | `SOULSTREAM.TOPICS.OPS.<topic-path>` | JetStream **OrderedConsumer** on stream `SOULSTREAM`, `FilterSubjects: [OPS.<path>]`, `DeliverAllPolicy` — history then live in one continuous stream, baseline first | `topic/follow.go:25-28` ✔, `topic/subjects.go:12,23` ✔, `realm/spec.go:14-19` ✔ |
| **"Leave"** | — | Stop the consumer. No server-side op, no membership state to clean up | `docs/topic.md:20-21` ✔ |
| **Post a turn** | `OPS.<path>` | Publish op type `turn.post`, payload `TurnPayload{body, mentions?}`; `@mention`s parsed from body fire one `mention.notify` each to `SOULSTREAM.PERSONA.NOTIFY.<persona>` | `topic/vocab.go:13,88-92`, `topic/post.go:36-43`, `topic/mention.go:31-42` |
| **Note (lightweight contribution)** | `OPS.<path>` | Op type `comment.add`, payload `CommentPayload{body, anchor{kind:"op", op_id}, mentions?}` — anchored to an existing op | `topic/vocab.go:14,100-105`, `topic/post.go:47-56` |
| **Start a topic** | `INFO.<path>` + `OPS.<path>` | Two publishes: `topic.announce` → `AnnouncePayload{topic_id, name, subject_matter?, expected?, tags?, parent?}` on INFO; then the initial `baseline` → `BaselinePayload{state, frontier}` as the **first** OPS message (inline state ≤ 128 KB) | `topic/start.go:29-69,14`, `topic/vocab.go:47-55,62-67`, `hq/02-DESIGN/core/03-topics.md:33` ✔ |
| **Close a topic** | `OPS.<path>` | Op `life.transition` with `TransitionPayload{to:"closed"}`; closed is "not writable by convention", not enforced. Lifecycle states are derived from the log, never stored | `topic/lifecycle.go:48-60`, `topic/vocab.go:30-45,107-111`, `topic/post.go:23-24` |
| **Inbox read** | `SOULSTREAM.PERSONA.NOTIFY.<persona>` | OrderedConsumer over stream `SOULSTREAM_NOTIFY`; bounded window (`MaxMsgsPerSubject: 100`, `DiscardOld`); **no ack, no cursor** — each fetch re-reads the window, consumer de-dupes on its side | `topic/notify.go:18-21,57-124`, `realm/spec.go:25-31,67-79` ✔ |
| **Discovery** | `SOULSTREAM.SVC.DISCOVER` | Core NATS request-reply, scatter/gather, **no queue group** (every responder hears every request); asker-enforced deadline, zero responders → empty result, not error | `topic/subjects.go:17-19` ✔, `topic/discover.go:96-128,199-242` |
| **Board read** | `SOULSTREAM.TOPICS.INFO.>` | `stream.Info` with subject filter + `GetLastMsgForSubject` per topic; board is one message per topic via per-subject rollup | `topic/board.go:29-79` |

## Wire obligations when writing

Headers on every op (`record/record.go:18-27`, `hq/02-DESIGN/core/01-protocol.md:77-85`):

- `Nats-Msg-Id` — writer-generated UUID, **held across retries**; JetStream
  dedup (`Duplicates: 2m`, `realm/spec.go:47,59`) makes publish idempotent.
- `Soulstream-Version` — wire version `1` (`record/record.go:14`); changes
  "only with a major version, expected to be rare to never"; vocabulary
  evolution is additive, unknown op types are ignored with a warning
  (`01-protocol.md:78,82`, `topic/vocab.go:8-9`).
- `Soulstream-Author` — must equal the connection's persona; the client library
  refuses to post as anyone else (`realm/connect.go:101-106`,
  `topic/wire.go:48-52` — a persona is *required* to write, optional to read).
- `Soulstream-Parents` — the writer's observed frontier (leaf op-ids). Stale or
  forked parents are merge-safe (eg-walker CRDT, `03-topics.md:86-97`), so
  best-effort is legal, but clean parenting means materialising first.
- `Soulstream-Ts` — informational only; ordering authority is DAG + stream
  sequence + author tie-break (`01-protocol.md:81`).
- `Soulstream-Sig` — **optional** Ed25519 over the RFC-8785 canonical record
  `{v, realm, topic, id, author, parents, ts, type, data}` where `topic` is
  derived from the subject by `canonicalBinding` (`topic/subjects.go:33-48` ✔,
  `record/canonical.go:40-81`). Verification is a read-side annotation, never a
  gate.

## Realm scoping and identity

- The realm is **not a subject token** — one NATS account per realm; tenancy is
  the account boundary (`01-protocol.md:11`). An imp scopes by which NATS
  context it connects with (`realm/connect.go:40-51`, via
  `natscontext`).
- Identity is stateless: persona = a name (+ optional key). **No login, no
  session, no handshake, no heartbeat, no server-side participant state
  anywhere in topic participation.**
- The realm must be provisioned once by an operator (`realm/provision.go:30-66`
  creates the `SOULSTREAM` stream, `SOULSTREAM_NOTIFY` stream,
  `soulstream-objects` object store, `soulstream-personas` KV).

## Protocol state a consumer is forced to carry (feeds Bar 2 / reversal)

1. **Frontier per topic** (write-side): parent new ops on observed leaves.
   Merge-safe if stale — not a correctness obligation, a quality one.
2. **Baseline-first replay** (read-side): first OPS message is the baseline;
   an unresolvable manifest baseline poisons the view (`topic/follow.go:75-88`).
3. **Idempotency token**: generate the op-id once, reuse on retry.
4. **Inbox de-dup**: no cursor protocol; repeated fetches re-see the window.

Nothing here is a protocol state *machine* — no sequencing handshake, no acks,
no session. Rollup/compaction is explicitly optional for correctness
(`03-topics.md:112`, `topic/rollup.go:36-42`); a plain read/post participant
has no rollup duty. Work-claim (`work.claim`) is publish-then-materialise —
the log decides the winner (`topic/work.go:88-92`) — relevant only if imps
adopts work items, not for M1 participation.

## Known doc/code divergence (code wins)

`01-protocol.md:21` still describes one stream capturing `SOULSTREAM.>`.
Shipped code (feature 014) splits it: the op-log captures only
`SOULSTREAM.TOPICS.>`, persona inboxes live in `SOULSTREAM_NOTIFY`, and
`SOULSTREAM.SVC.>` is deliberately captured by **no** stream
(`realm/spec.go:16-31` ✔, including the legacy-shape auto-convergence note).
