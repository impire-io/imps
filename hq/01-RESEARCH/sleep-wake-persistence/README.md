# Can sleep, wake, and persistence ride the existing state seam?

**State:** active
**Started:** 2026-07-26

## Abstract

M2 — sleep/wake and snapshot persistence — is the roadmap's front, and its
gate demands boundaries before mechanisms: the eviction/rehydration boundary
and the snapshot/restore contract specified *before* any backend is chosen.
This topic pins that boundary on the shipped harness's per-entity state seam
(the registry behind `State(name, entity)`), specifies the wake-hook
semantics as a contract, and measures both with a spike against a reference
backend (NATS KV on an embedded server — a stand-in, deliberately not a
commitment). A decisive answer graduates into the `02-DESIGN/` doc that
clears M2's gate; a negative answer redirects M2 to a harness-native memory
redesign before any code is written. The M1 precedent (episode 0004) makes
the cheap hypothesis explicit: participation needed zero harness changes —
this topic tests whether persistence enjoys the same luck or names exactly
the minimal hooks it needs.

## The question

Can per-entity persistence — eviction under the existing slot bound,
snapshot/restore across imp restarts, and a wake-hook carrying true elapsed
time — be expressed as a contract on the imp's existing per-entity state
seam, with at most named minimal additions to the harness, validated against
a reference backend without committing the framework to any backend?

## Pre-registered bars

- **Bar 1 — the seam inventory is complete and pinned.** Every hook point
  that eviction, rehydration, snapshot, restore, and wake need is identified
  in the shipped harness with `file:line` evidence, and each is classified
  as *already-sufficient*, *minimal-addition* (exact signature named), or
  *blocking*. *Pass:* zero hooks classified blocking. *Fail:* any needed
  hook requires restructuring the dispatch path or the awareness/thinking
  context surfaces.
- **Bar 2 — a snapshot/restore round-trip holds on the specified boundary.**
  A scratchpad spike: an imp with a codec-equipped state shape mutates
  per-entity state; the state is snapshotted to the reference backend; the
  imp stops; a fresh imp instance starts; on next access the entity's state
  rehydrates **equal under the codec** to the pre-stop value. *Pass:*
  equality measured, with harness-core changes limited to Bar 1's named
  minimal additions (zero preferred). *Fail:* equality breaks, or the spike
  needs harness changes Bar 1 did not name.
- **Bar 3 — the wake-hook fires exactly once with true elapsed time.** The
  spike gives an entity time-dependent state; after a stop of measured
  duration, the wake signal delivers the elapsed interval to user code
  exactly once per entity wake, and the state advances deterministically
  from it. *Pass:* exactly-once and elapsed-within-tolerance measured.
  *Fail:* double-fire, missed fire, or an elapsed value the recorded
  contract cannot produce.
- **Bar 4 — eviction keeps memory bounded without loss.** The spike touches
  more entities than the shape's `Cap`; resident slots never exceed the
  bound, and every touched entity's state remains retrievable afterward
  (rehydrated from the backend). *Pass:* bound held and zero loss. *Fail:*
  the bound breaks, state is lost, or boundedness is achievable only by
  rejecting new entities.

## Reversal condition

If Bars 2–4 can only pass by intercepting every state access inside the
harness dispatch path or by replacing the registry wholesale — that is, Bar 1
finding a *blocking* classification the spike confirms — the glue-first
direction reverses: M2 becomes a harness-native memory feature, and the
design doc records the boundary as owned by the harness rather than layered
beside it.

## Verdict

**Answer: yes — beside the registry, not inside it.** All four bars passed on
2026-07-26, the topic's opening day, with **zero harness changes** (stronger
than the pre-registration, which held minimal additions in reserve).

- **Bar 1 — PASS `[measured]`.** The seam inventory (JOURNEY.md table) pins
  every needed hook with `file:line` evidence. The registry-riding route is
  blocking four ways at once (cap-rejection contract, error-less `Get`,
  entity-less `Factory`, no enumeration) — but the boundary doesn't need the
  registry: `Entity` + user code + a backend are already-sufficient, so zero
  hooks in the *chosen* route are blocking and none were even needed.
- **Bar 2 — PASS `[measured]`.** The spike's restart round-trip: state
  mutated through a real imp's awareness (write-through to NATS KV as the
  reference backend), imp stopped, fresh instance rehydrated `Counter == 6`
  equal under the codec. 3 consecutive `-race` runs; imps tree
  byte-identical.
- **Bar 3 — PASS `[measured]`.** Wake fired exactly once per rehydration,
  elapsed ≥ the 400 ms slept and wall-clock-bounded, state advanced as a
  pure function of the delivered elapsed; resident re-access did not
  re-fire.
- **Bar 4 — PASS `[measured]`.** 10 entities through a bound of 4: residency
  never exceeded the bound, zero state loss (write-through makes eviction a
  lossless drop by construction).

**Reversal condition: not triggered** — no blocking hook was confirmed
because the chosen boundary never touches the registry. The adversarial pass
(JOURNEY.md) resolved placement to an `imps/persist`-shaped glue module
`[judgment]`, with a registered follow-on reversal: real two-tier
inconsistency bugs or measured dispatch-latency damage from bounded IO in
awareness moves the boundary into the harness.

**Graduation direction: design** — the snapshot/restore contract (one
envelope per entity: codec bytes + last-active stamp; write-through *is* the
snapshot), wake-hook semantics (per-entity on rehydration; imp-level as a
pre-`Run` gate), and the eviction/rehydration boundary are all specified and
measured; the backend stays uncommitted behind the reference implementation.
