# Who owns sleep: where does the imps/soulrealm boundary sit for sleep, wake, and persistence?

**State:** active
**Started:** 2026-07-27

## Abstract

Feature `005-sleep-wake-persistence` (PR #8, now held in draft) implemented
M2 inside imps — but the owner's expectation was that sleep/wake would be
handled by **soulrealm**, the realm's runtime (launch, supervise, retire,
pluggable isolation backends). The M2 research (episode 0005) inventoried the
imps seam but never inventoried the sibling project's intended scope — a
stakeholder gap this topic exists to close. The investigation pins both
projects' declared intents from their own hq documents, assigns each of M2's
constituent concerns an owner through discriminating arguments (not
preference), and delivers a disposition for every surface PR #8 shipped. The
outcome decides what of feature 005 survives, in what form, and what the
imps↔soulrealm integration seam is.

## The question

For each concern M2 bundled — whole-imp suspend/resume, the authoritative
slept-for signal, per-entity durable state across restarts and redeploys,
bounded residency with eviction/rehydration, and the advance-by-elapsed wake
contract in imp code — which project owns it, by an argument the other side
cannot answer, and what happens to each surface of feature 005 (`Store`,
`Beacon`, the wake hook) as a result?

## Pre-registered bars

- **Bar 1 — both scopes pinned to their owners' documents.** For each of the
  five concerns, what the imps hq (vision, anatomy, constitution) and the
  soulrealm hq (vision, constitution, design docs, roadmap — plus the
  soulstream work extension it builds on) actually declare, with `file:line`
  evidence — including documented *absence* where a project is silent.
  *Pass:* every concern has cited evidence or cited absence from both repos.
  *Fail:* any concern characterized from memory or vibes.
- **Bar 2 — every assignment carries a discriminator.** Each concern's owner
  is decided by an adversarial pass argued at full strength both ways, and
  the winning side names a *discriminator* — something the losing owner
  structurally cannot do (not "fits better"). *Pass:* five assignments, five
  discriminators. *Fail:* any assignment resting on preference or symmetry.
- **Bar 3 — a complete disposition for feature 005.** Every shipped surface
  (`persist.Store`, `persist.Beacon`, the per-entity wake hook, the docs the
  landing changed) is classified keep-as-is / keep-reframed /
  move-to-soulrealm / delete, with the exact documents that must change
  enumerated, and the imps↔soulrealm integration seam named (how the
  runtime's wake signal reaches imp code, even if soulrealm builds its side
  later). *Pass:* no surface left undecided, seam contract sketched.
  *Fail:* any "to be determined" in the disposition.

Because this topic exists to correct a direction call made without a
teach-back, its verdict is **presented to the owner for teach-back before
graduation** — the working agreement applied to the very failure that opened
the topic.

## Reversal condition

Two, registered now: (1) if soulrealm's hq later declares per-entity state
services in its own scope (a design doc claiming durable agent-state
management as a runtime concern), the imp-side assignment of the durable
tier reopens; (2) if a real deployment shows the runtime cannot deliver an
authoritative slept-for signal across its isolation backends (evidence: a
backend where suspend/resume elapsed is unknowable), the stopgap-vs-contract
framing of the Beacon reverses toward the Beacon as the permanent source.

## Verdict

**Answer: the boundary is a triangle, and feature 005 built the right thing
under the wrong name.** All three bars passed on 2026-07-27, and the
disposition survived the owner's teach-back (2026-07-27, this session) with
the trade-offs laid out explicitly before acceptance.

- **Bar 1 — PASS `[measured]`.** Both scopes pinned from the owners'
  documents (JOURNEY.md): soulrealm is *silent* on suspend/resume,
  slept-for, and workload-internal memory, and **constitutionally disclaims
  durable state** (Article I: never a store of record — that belongs to
  soulstream); imps' vision owns only the *contract* ("the imp doesn't know
  it was asleep"; the isolation mechanism is explicitly outside the
  framework).
- **Bar 2 — PASS.** Five assignments, five structural discriminators
  (JOURNEY.md): suspend/resume mechanism and the authoritative slept-for
  source → soulrealm's future backend seam (a process cannot snapshot
  itself; no self-stamp is authoritative when no imp code runs at suspend);
  per-entity durable state, bounded eviction, and advance-by-elapsed code →
  imps (soulrealm's constitution forbids the first `[measured]`; redeploys
  make the second irreducible `[mechanism-argument]`; only imp code can
  advance application state).
- **Bar 3 — PASS.** Complete disposition, no surface undecided:
  `persist.Store` + per-entity wake **keep-as-is**; `persist.Beacon`
  **keep-reframed** as the restart clock (self-reported; interim source,
  not the sleep signal); the 005 landing's anatomy claims **rewritten**
  (isolation-snapshot sleep restored as the runtime-owned common case,
  imp-level snapshot wake honestly `[D]` — it needs mid-process delivery
  co-designed with the runtime); milestone **split into M2a** (durable
  memory tier — ships) **and M2b** (snapshot sleep/wake, gated on soulrealm
  growing suspend/resume plus a co-designed wake-delivery contract). The
  integration seam is sketched in JOURNEY.md.

**Process note `[judgment]`:** the topic exists because a direction call was
made without a teach-back; its verdict was therefore withheld for one, and
the accepted trade-off analysis (including that the `Backend` boundary keeps
the durable tier's storage home movable later) is part of the record.
