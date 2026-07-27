# Episode 0007 — Who owns sleep: the boundary challenge that split M2 (2026-07-27)

Before PR #8 merged, the owner challenged feature 005's premise: "I expected
this to be handled by soulrealm instead of an explicit feature in the imps."
The PR went to draft and the `sleep-boundary-with-soulrealm` research topic
opened to answer it properly — because the M2 research had inventoried the
imps seam but never the sibling project's scope, and the 005 landing had
rewritten the anatomy's sleep story without a teach-back. This topic's
verdict was withheld for exactly that teach-back, and survived it with the
trade-offs on the table.

**Bar 1 (both scopes pinned) — PASS `[measured]`.** Soulrealm's own hq is
*silent* on suspend/resume, sleep, snapshots, slept-for, and
workload-internal memory (zero hits across every doc and all code; its
lifecycle has terminal states only, and even bounded auto-restart is "a
named later feature") — and it **constitutionally disclaims durable state**:
Article I, non-negotiable, "Soulrealm is a runtime, never a store of
record… No feature may make soulrealm the place a piece of durable truth
lives" — that home is *soulstream*. The imps vision owns only the contract:
"the imp doesn't know it was asleep… what the framework specifies is the
contract that any isolation mechanism satisfies." The boundary is a
**triangle** — soulstream the record, soulrealm the room, imps the
inhabitant — and the M2 research had drawn it as a line.

**Bar 2 (assignments by discriminator) — PASS.** Five concerns, five
structural discriminators: whole-imp suspend/resume → **soulrealm's** future
backend seam (a process cannot snapshot its own image); the authoritative
slept-for → **soulrealm supplies, imps contracts** (no self-stamp is
authoritative when no imp code runs at suspend time); per-entity durable
state → **imps** (soulrealm's constitution forbids owning it `[measured]`,
and redeploys make imp-side serialization irreducible — a new binary cannot
resume an old memory image `[mechanism-argument]`); bounded eviction →
**imps** (requires per-entity knowledge a supervisor cannot have);
advance-by-elapsed code → **imps** (only imp code can advance application
state). Considered and rejected: persisting private imp state into
soulstream topics — the "everything worth keeping flows back as ops"
doctrine governs collaboration artefacts, not an imp's private interpretive
cache.

**Bar 3 (disposition) — PASS, applied in this same merge.** Feature 005
built the right thing under the wrong name: `persist.Store` and the
per-entity rehydration wake **keep as-is**; the `Beacon` is **reframed** as
the *restart clock* — self-reported elapsed for stops, deploys, and
heartbeat-bounded crashes, the interim imp-level source, explicitly not the
sleep signal (a snapshot-resume continues mid-`Run`; the pre-`Run` gate
never re-executes — that mid-process wake surface is honestly unbuilt);
the 005 landing's anatomy rewrite is **rolled back** where it recast
"stopping is sleeping" as the sleep story; and the milestone **splits**:
**M2a** (durable tier + restart clock — shipped by 005) and **M2b**
(whole-imp snapshot sleep/wake, `[D]`, gated on soulrealm declaring
suspend/resume plus a co-designed wake-delivery contract, seam sketched in
the topic journey).

**What was refuted:** the tacit assumption that "handled by soulrealm" and
"handled by imps" were the only two readings — soulrealm's own constitution
routes durable truth to soulstream, and its runtime scope today ends at
launch/supervise/retire. Also refuted, in the other direction: 005's
"stopping is sleeping" framing, which had quietly demoted the vision's
common case. **What it taught:** boundary research must inventory *every*
sibling's declared scope, and the working agreement's teach-back is the
cheapest correction mechanism the process has — the challenge cost one day;
silent drift would have cost the anatomy its truthfulness.

Reversal condition: two, registered in the topic and carried here — (1) a
soulrealm hq design doc claiming durable agent-state management reopens the
imp-side assignment of the durable tier; (2) an isolation backend where
suspend/resume elapsed is unknowable to the runtime reverses the
Beacon-as-interim framing toward the Beacon as the permanent imp-level
source. Additionally: the M2b gate is external — if soulrealm's roadmap
never grows suspend/resume, "sleep is the common case" must be openly
re-examined at the vision level rather than left aspirational forever.

Trail: [`../02-DESIGN/0004-sleep-wake-persistence.md`](../02-DESIGN/0004-sleep-wake-persistence.md)
(re-titled "Per-Entity Persistence and the Restart Clock", M2b exclusion
stated); anatomy Persistence-and-sleep and wake-hook sections re-split
`[V]`/`[D]`; roadmap M2a/M2b; the topic's pre-registration, scope audit,
adversarial pass, and teach-back record live in git history under
`hq/01-RESEARCH/sleep-boundary-with-soulrealm/` (removed at graduation);
commits `6c945a3`, `b0c876b`; PR #8 (held in draft during the topic,
re-readied with this change).
