# Can soulstream topic participation ride the existing channel seam?

**State:** active
**Started:** 2026-07-25

## Abstract

M1 on the roadmap — soulstream coordination channels — is the next declared
milestone, and its gate needs two things this topic exists to produce: the
soulstream service's subject contract (owned by the `soulstream` repo; this
framework only consumes it) and evidence that topic participation fits the
harness as it is. The investigation pins that contract to its owner and tests
whether joining a topic can be an ordinary channel-add on the existing dispatch
seam, with awareness's surface growing by at most `Note`. A decisive answer
graduates into the `02-DESIGN/` doc that clears M1's gate for
`/speckit-specify`; a negative answer redirects M1's shape before any harness
code is written.

## The question

Can an imp join, contribute to (turns from thinking, notes from awareness), and
leave soulstream topics purely by consuming the soulstream service's subject
contract — with topic membership expressed as channel add/remove on the
existing dispatch seam, and awareness gaining at most `Note`?

## Pre-registered bars

- **Bar 1 — the contract is pinned to its owner.** Every subject, payload
  shape, and lifecycle rule the design consumes (join, post turn, note, leave,
  and whatever the service requires around them) is recorded in a contract
  table where each row cites its owning handler in the `soulstream` repo's
  source or a recorded request/reply against a live soulstream service.
  *Pass:* zero consumed subjects resting on guesswork. *Fail:* any row without
  owning-repo or live-service evidence.
- **Bar 2 — topics ride the seam unchanged.** A scratchpad spike wires a topic
  subscription as a channel against embedded NATS, stubbing the soulstream
  subjects exactly per the Bar 1 table. *Pass:* topic messages dispatch through
  the existing channel seam with the harness's dispatch code byte-identical (no
  new dispatch branch, no contract change), demonstrated by the spike running
  green. *Fail:* the spike needs any modification to the dispatch contract.
- **Bar 3 — awareness stays bounded.** In the proposed API shape, awareness
  gains at most `Note`; no topic open/close/leave method lands on
  `AwarenessContext`. *Pass:* `make compile-deny` green against the spike's API
  shape and zero topic-lifecycle methods on `AwarenessContext`. *Fail:*
  otherwise.

## Reversal condition

If the soulstream contract requires stateful handshakes, per-topic ordering, or
server-driven membership that the spike can only satisfy by adding a new
dispatch path or a protocol state machine inside the harness — Bar 2 failing
for contract reasons, not implementation reasons — the direction reverses:
topic participation becomes an external capability client per
[`../../02-DESIGN/0002-capability-service-pattern.md`](../../02-DESIGN/0002-capability-service-pattern.md)
rather than a channel kind, and M1 is redrafted accordingly.

## Verdict

**Answer: yes, and more cheaply than pre-registered — participation needs zero
harness changes.** All three bars passed on 2026-07-25; the investigation ran
one day.

- **Bar 1 — PASS `[measured]`.** The contract table ([contract.md](contract.md))
  covers every consumed subject/operation with `file:line` evidence in the
  owning repo; zero rows on guesswork; load-bearing rows independently
  spot-checked against the cited source. Headline: the protocol has **no
  join/leave and no membership state** — participation is subscribe + publish
  (`docs/topic.md:20-21`, `03-topics.md:29`, no join/leave in `topic/vocab.go`).
- **Bar 2 — PASS `[measured]`.** The scratchpad spike (JOURNEY.md, 2026-07-25)
  read a live topic through the **existing** `StreamSource` channel — stream
  `SOULSTREAM`, filter `OPS.<path>`, `DeliverAllPolicy`: 7/7 ops in stream
  order, baseline first, history→live over one continuous consumer, ephemeral
  consumer deleted on shutdown. Three consecutive `-race` runs (~0.5 s each);
  `git status --porcelain` empty in both working trees — the dispatch code was
  byte-identical, not merely contract-compatible.
- **Bar 3 — PASS `[measured]`.** Awareness gained **nothing** (stronger than
  the registered "at most `Note`"): the `Note` verdict already exists, and the
  emission is the `OnNote` hook posting `comment.add` anchored to the observed
  op via the owner's library. `make compile-deny` green on the unchanged tree;
  zero topic-lifecycle methods on `AwarenessContext`.

**Reversal condition: not triggered `[measured]`** — no handshake, session,
ack protocol, or state machine exists anywhere in topic participation; the
consumer-side state is a best-effort frontier (empty is legal and was
exercised) and the baseline-first replay rule, both absorbed by existing
channel semantics.

**Graduation direction: design.** The adversarial pass (JOURNEY.md) resolved
library-vs-wire in favour of a glue package outside the harness core
(`[judgment]`, reversal condition registered: a real scenario requiring an
imp's topic set to change without restart reopens runtime channel lifecycle).
