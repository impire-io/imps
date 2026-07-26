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

<Empty until graduation. Filled by /research-graduate: PASS/FAIL per bar with
the honest numbers, each load-bearing claim tagged [measured] /
[mechanism-argument] / [judgment].>
