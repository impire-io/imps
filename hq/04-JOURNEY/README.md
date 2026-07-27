# 04-JOURNEY — the narrative record

What was built, what was measured, what was believed and then refuted, and what
each episode taught. Specs say what the framework *is*; these episodes say how
we *got here* — including the dead ends, because the refuted assumptions are as
load-bearing as the shipped code.

> **Keeping this log alive:** whenever a feature lands, a research investigation
> concludes, or a load-bearing decision is made, add a numbered episode with
> `/journey-log` (research topics get theirs via `/research-graduate`). Follow
> [`TEMPLATE.md`](TEMPLATE.md) — including its required Reversal-condition line
> and evidence-class tags. Honesty rules apply here as everywhere: record what
> actually happened, including failures, reversals, and findings that
> contradicted expectations. This duty is anchored in
> [`../00-GENESIS/how-we-work.md`](../00-GENESIS/how-we-work.md); the numbering
> and index are enforced by `internal/hqlint`.

## Where things stand (2026-07-27)

**The framework's core is shipped, and the project just moved into `hq/`.** The
in-process Go substrate — channels (core-subject + JetStream), awareness
dispatch, thinking in its own goroutine, per-entity local memory, and the
outbound NATS surface (`Request` / `RequestMany` / `Publish` / `Conn`) — landed
across features `001-harness-core` and `002-capability-client`, with the
awareness/thinking boundary **compile-enforced** by the `integration/compiletest/`
build-tag assertions (`make compile-deny`). A follow-on sweep flattened the
package to the module root and renamed `Wake`→`Think` and `Reasoning`→`Thinking`,
and the action whitelist was removed in favor of substrate ACLs plus the
compile-enforced boundary — all recorded, with the frozen spec-vs-code drift, in
[episode 0001](0001-founding-the-harness.md).

**How the project is run now lives in `hq/`** ([episode 0002](0002-hq-adoption-and-mit.md)):
GENESIS holds the vision, the constitution (v2.3.0, wired into every spec-kit
plan via the `.specify/memory/constitution.md` symlink, now carrying the
anti-drift Working Agreement), and how-we-work; research runs a
`/research-start` → `/research-graduate` lifecycle; this journal is numbered
episodes with the structure enforced by `internal/hqlint`; the license is
**MIT**.

**M1 is shipped** ([episode 0003](0003-soulstream-participation.md) →
[episode 0004](0004-soulstream-participation-shipped.md)): on 2026-07-25 the
`soulstream-participation` research topic ran its pre-registered bars,
graduated to design, and the feature (`004-soulstream-participation`) landed —
research to shipped module in one day. The soulstream protocol has no
join/leave — presence is the consumer — so participation ships as the
`imps/soulstream` **nested glue module** (owner library pinned at v0.4.0):
`TopicChannel` reads a topic through the existing `StreamSource`, the
`NoteBridge` turns the shipped `Note` verdict into anchored `comment.add`
contributions, and `Participant` gives thinking the full write path on the
imp's own connection. The harness core's diff for the whole feature is
**zero** — `go.mod` byte-identical, compile-deny green — and the research
spike is now the permanent integration suite.

**M2a is shipped, and the boundary that re-scoped it is settled**
([episode 0005](0005-sleep-wake-persistence.md) →
[episode 0006](0006-sleep-wake-persistence-shipped.md) →
[episode 0007](0007-sleep-boundary-with-soulrealm.md)): on 2026-07-26 the
`sleep-wake-persistence` research topic passed its four pre-registered bars
and feature `005-sleep-wake-persistence` was built; before it merged, the
owner's challenge ("I expected soulrealm to handle this") opened the
`sleep-boundary-with-soulrealm` topic, whose verdict — survived teach-back —
split the milestone. **M2a** (shipped by 005): the durable memory tier
beside the registry as the **`imps/persist` package** (zero new
dependencies, zero root-package changes): a write-through, bounded-LRU
store (`DefaultBound = 256`) with rehydration-on-access, a per-entity wake
fired exactly once per rehydration with true elapsed time, lossless
eviction by construction, and the `Beacon` **restart clock** as the
interim imp-level elapsed source. **M2b** (new, `[D]`): whole-imp snapshot
sleep/wake — the runtime's act; soulrealm's hq is today silent on
suspend/resume and constitutionally disclaims durable state (that belongs
to soulstream), so M2b is gated on soulrealm declaring suspend/resume plus
a co-designed mid-process wake-delivery contract. The anatomy now carries
the honest split: restart path `[V]`, snapshot path `[D]`. Remaining
`[D]`: schedule channels (M3 — the per-entity wake semantics it needs are
settled), audit emission (M4), and M2b
([`../03-IMPLEMENTATION/roadmap.md`](../03-IMPLEMENTATION/roadmap.md)).
There are no active research topics.

## Episode index

| # | Episode |
|---|---|
| 0001 | [Founding the harness: the substrate, then the sweep](0001-founding-the-harness.md) |
| 0002 | [The project moves into HQ, and picks a license](0002-hq-adoption-and-mit.md) |
| 0003 | [Soulstream participation: the join that isn't there](0003-soulstream-participation.md) |
| 0004 | [M1 ships: soulstream participation as a glue module](0004-soulstream-participation-shipped.md) |
| 0005 | [Sleep, wake, persistence: beside the registry, not inside it](0005-sleep-wake-persistence.md) |
| 0006 | [M2a ships: the durable tier, transcribed from measurement](0006-sleep-wake-persistence-shipped.md) |
| 0007 | [Who owns sleep: the boundary challenge that split M2](0007-sleep-boundary-with-soulrealm.md) |
