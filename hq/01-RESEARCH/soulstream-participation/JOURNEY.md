# Journey — soulstream-participation (started 2026-07-25)

## 2026-07-25 — Bar 1: the contract, pinned from the owning repo

Swept the `soulstream` repo (local checkout, `main`, clean) with a read-only
survey agent, then independently spot-checked the load-bearing citations by
reading the cited lines directly (`topic/subjects.go`, `topic/follow.go`,
`docs/topic.md`, `realm/spec.go`, `hq/02-DESIGN/core/03-topics.md`). Every
spot-check matched the survey verbatim. Result: [`contract.md`](contract.md),
one row per consumed subject/operation, each with `file:line` evidence
`[measured]` (readings in the owning repo's source and normative docs).

**The finding that reshapes M1:** soulstream has **no join/leave operation and
no membership state**. "Joining" a topic is subscribing (a JetStream ordered
consumer filtered to `SOULSTREAM.TOPICS.OPS.<path>` — history then live,
baseline first); "leaving" is stopping the consumer; posting rights are subject
permissions, nothing else `[measured]` (`docs/topic.md:20-21`,
`03-topics.md:29`, zero join/leave types in `topic/vocab.go`). This is exactly
the channel-add/channel-remove shape M1 hypothesised — the harness would not be
modelling membership at all.

**Natural `Note` mapping found:** `comment.add` anchored to an op-id — a
lightweight contribution that annotates an existing op without opening
anything. Awareness observes a dispatched op and can note *on that op*; the
anchor is the op it just saw. `turn.post` remains the thinking-tier
contribution. `[mechanism-argument]` — the mapping is argued from the
vocabulary's shape, not yet exercised.

**No protocol state machine found:** no session, handshake, heartbeat, ack, or
cursor anywhere in topic participation. The consumer-side state is a per-topic
frontier (merge-safe if stale) and the baseline-first replay rule on read
`[measured]` (contract.md, "Protocol state" section). On today's evidence the
reversal condition is not triggered.

**Open questions carried forward to Bar 2:**

1. **Library vs. wire:** consume soulstream's own Go client
   (`github.com/impire-io/soulstream/topic` — `Handle`, `Follow`, `PostTurn`,
   `AddComment` already exist) or speak the wire from imps directly? The
   library route imports the capability owner's code into the harness; the
   wire route re-implements headers, canonical signing binding, and frontier
   logic that the owner already maintains. Constitution lens: "capabilities
   are external" cuts both ways here — soulstream is substrate coordination
   (vision: "coordination happens through the soulstream"), not a capability
   service. Needs the adversarial pass before the design doc.
2. **Where frontier state lives** — in the channel, or in per-entity local
   memory keyed by topic?
3. **Whether an imp's topic read rides the existing JetStream channel kind
   unchanged** — the existing channel kind vs. the ordered-consumer,
   baseline-first shape soulstream reads need. This is precisely Bar 2's
   spike.
