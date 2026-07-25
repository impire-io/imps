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

## Where things stand (2026-07-25)

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

**M1's gate is cleared** ([episode 0003](0003-soulstream-participation.md)):
the first research topic, `soulstream-participation`, ran its pre-registered
bars and graduated to design in one day (2026-07-25). The soulstream subject
contract is pinned to its owning repo, and the spike proved participation
needs **zero harness changes** — the protocol has no join/leave, a topic reads
as the existing `StreamSource` channel, and the shipped `Note` verdict bridges
to `comment.add` contributions. The design,
[`../02-DESIGN/0003-soulstream-participation.md`](../02-DESIGN/0003-soulstream-participation.md),
specifies M1 as a **glue module** (`imps/soulstream`, own `go.mod`) and is
ready for `/speckit-specify`. The remaining milestones (sleep/wake + snapshot
persistence, schedule channels, audit emission) stay `[D]`, sequenced
gates-not-dates in
[`../03-IMPLEMENTATION/roadmap.md`](../03-IMPLEMENTATION/roadmap.md). There
are no active research topics.

## Episode index

| # | Episode |
|---|---|
| 0001 | [Founding the harness: the substrate, then the sweep](0001-founding-the-harness.md) |
| 0002 | [The project moves into HQ, and picks a license](0002-hq-adoption-and-mit.md) |
| 0003 | [Soulstream participation: the join that isn't there](0003-soulstream-participation.md) |
