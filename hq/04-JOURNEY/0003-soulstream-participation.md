# Episode 0003 — Soulstream participation: the join that isn't there (2026-07-25)

The M1 gate needed two things: the soulstream service's subject contract,
pinned to its owner, and evidence that topic participation fits the harness as
it is. The research topic `soulstream-participation` pre-registered three bars
and passed all three in one day — and the answer came in cheaper than the
question assumed: **participation needs zero harness changes.**

**Bar 1 (contract pinned) — PASS `[measured]`.** Every subject, payload, and
wire obligation an imp participant consumes was traced `file:line` into the
owning `soulstream` repo, load-bearing rows independently spot-checked against
the cited source. The headline reading: the protocol has **no join, no leave,
and no membership state**. "You don't 'join' a topic like a chat room — you
just start writing in the notebook" (`docs/topic.md:20-21`); posting rights are
subject permissions, nothing else; no join/leave type exists in the vocabulary.
Joining *is* subscribing; leaving *is* stopping.

**Bar 2 (topics ride the seam unchanged) — PASS `[measured]`.** A scratchpad
spike compiled against the imps working tree via a `replace` directive and ran
the full loop on embedded NATS provisioned by soulstream's own code: an
**existing** `StreamSource` channel (stream `SOULSTREAM`, filter
`SOULSTREAM.TOPICS.OPS.<path>`, `DeliverAllPolicy`) delivered 7/7 ops in
stream order — baseline first, two history turns, then live turns and comments
over one continuous consumer with no history/live seam. Three consecutive
`-race` runs (~0.5 s each); `git status --porcelain` empty in both working
trees throughout — the dispatch code was byte-identical, not merely
contract-compatible. Shutdown deleted the ephemeral consumer, which is all the
protocol means by leaving.

**Bar 3 (awareness stays bounded) — PASS `[measured]`.** Awareness gained
*nothing* — stronger than the registered "at most `Note`". The `Note` verdict
already existed; the spike bridged it in the `OnNote` hook, which posted
`comment.add` anchored to the observed op via the owner's library
(`realm.NewClient` wrapping the imp's own connection), and the contributions
round-tripped back through the imp's own channel, visible as first-class
anchored comments in the owner's materialised view. `make compile-deny` green
on the unchanged tree.

**Refuted along the way:** the anatomy's placeholder subject shape
(`$soulstream.*` — the real namespace is `SOULSTREAM.TOPICS.*`, realm scoping
by NATS account, not subject token) and the assumption that M1 would need a
new channel kind or a protocol-level join/leave op. The topic's registered
reversal condition — a contract forcing protocol state into the harness — was
**not triggered**: no handshake, session, ack, or cursor exists anywhere in
topic participation `[measured]`.

**What it opened:** the adversarial pass on library-vs-wire resolved to a
**glue package outside the harness core** (`[judgment]`; the wire's read path
is three header reads, the write path belongs to the owner's library — dep
cost measured at `uuid`+`jcs`+`natscontext`). M1 becomes a glue-module
feature, not a harness feature, specified in
[`../02-DESIGN/0003-soulstream-participation.md`](../02-DESIGN/0003-soulstream-participation.md).
Dynamic runtime join/leave is deliberately deferred.

Reversal condition: the first real colony scenario in which the set of topics
an imp participates in must change without a restart — that reading reopens
runtime channel lifecycle (add/remove on the existing per-channel bind/teardown
seam) as a harness milestone.

Trail: [`../02-DESIGN/0003-soulstream-participation.md`](../02-DESIGN/0003-soulstream-participation.md)
(the graduated design, including the pinned contract); the topic's
pre-registration, contract table, spike record, and adversarial pass live in
git history under `hq/01-RESEARCH/soulstream-participation/` (removed at
graduation); commits `3c99c21`, `be61de2`, `c3cbe26`.
