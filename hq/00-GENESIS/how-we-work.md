# How we work

The process companion to [`constitution.md`](constitution.md): the pipeline,
the lifecycles, the duties, and how all of it is enforced. `hq/README.md`
holds the one-screen map.

## The pipeline

```
question ──/research-start──▶ 01-RESEARCH/<slug>/     (state: active)
                                   │
                     /research-graduate <slug>
                        │            │           │
                     design       artifact    abandoned
                        │            │           │
                        ▼            │           │
              02-DESIGN/NNNN-*.md    │           │
                        │            ▼           ▼
               /speckit-specify   04-JOURNEY episode (always; folder removed)
                        │
                        ▼
        specs/NNN-*/ + code  ──landed──▶ /journey-log episode
                        │                      + roadmap.md updated
                        ▼
        design docs updated (behavioral changes propagate back)
```

Two hard boundaries:

- **Research never goes through spec-kit.** Spec-kit assumes you know what
  you're building; research exists to find out whether to build. Research uses
  the pre-registration method below.
- **Implementation always goes through spec-kit.** A design doc in
  `02-DESIGN/` is written to be the argument to `/speckit-specify`; the
  generated plan's Constitution Check reads GENESIS through the
  `.specify/memory/constitution.md` symlink.

## Research (`01-RESEARCH/`)

One folder per topic, created with `/research-start <slug>`. The folder's
`README.md` (from `../01-RESEARCH/TEMPLATE.md`) carries: Title, State
(`active | graduated | abandoned`), Abstract, the Question, and
**pre-registered bars** — the pass/fail criteria written *before* any
experiment runs. The folder's `JOURNEY.md` records the investigation as it
happens.

- **Method:** hypothesis → cheap discriminating experiment → verdict, one
  variable at a time. Experiment scripts live in the session scratchpad;
  conclusions, documents, and principled code changes land in git.
- **Always committed and pushed** — even work that will be abandoned. The
  point is a permanent trail; abandoned research keeps its full history in git
  after the folder is gone.
- **Ending:** `/research-graduate <slug> --to design|artifact|abandoned`
  composes the topic's journey into the next-numbered `04-JOURNEY/` episode
  (verdict, evidence tags, reversal condition included), creates or updates the
  design doc when the outcome is a design, and removes the topic folder in
  every case. An abandoned topic is a *result*, recorded with the same care as
  a success.

## Design (`02-DESIGN/`)

Numbered documents (`0001-…` onward) describing the framework's architecture at
the functional level — explicit enough that `/speckit-specify` can turn one
into a spec without guessing: the capability, its seams, its configuration
surface, its acceptance criteria. Every component carries a maturity tag
(`[V]` / `[D]` / `[O]`; see the area README). Every behavioral change made
during implementation propagates back into the design docs it touches — the
docs describe the framework as it *is*.

## Implementation (`03-IMPLEMENTATION/` + `specs/`)

`roadmap.md` is the live plan: milestones, exit criteria, and the gate each
milestone depends on. No dates — gates, not calendars. Features run the
spec-kit flow (`/speckit-specify` → clarify → plan → tasks → implement) on a
numbered feature branch; `specs/NNN-*/` artifacts freeze when the feature
lands. Landing a feature means: gate green, roadmap updated, journey episode
written, design docs propagated — in the same merge.

## Journey (`04-JOURNEY/`)

The append-only log: one numbered episode (`NNNN-slug.md`) per landed feature,
concluded research topic, or load-bearing decision — written with
`/journey-log` (or `/research-graduate`, which writes it for research). The
`TEMPLATE.md` requires: what happened with honest numbers, what was refuted or
reversed, evidence-class tags on load-bearing claims, and a **Reversal
condition** line. `README.md` carries the preamble, the episode index, and the
"Where things stand" summary — both refreshed with every episode.

## The working agreement (anti-drift)

The four correctives are constitution articles (see The Working Agreement
there); this is how they run day to day:

- **When to teach-back:** any decision that changes direction, scope, a spec
  contract, or a public claim. The assistant asks for the restatement; the
  decision is recorded only after it survives.
- **Tagging:** write `[measured]` / `[mechanism-argument]` / `[judgment]`
  inline where the claim is made — in conversation, in episodes, in design
  docs. For a framework, `[measured]` means a reading in the repo: a test, a
  benchmark (`dispatch_bench_test.go`), a compile-deny outcome, a byte-diff. If
  a debate is being closed by anything other than `[measured]`, stop and say so.
- **Reversal conditions:** phrased as observable evidence ("a benchmark showing
  X", "N features that needed Y"), not vibes. Written at decision time, never
  retrofitted.
- **Adversarial pass:** for framework-identity calls the assistant argues the
  other side at full strength *before* the decision — or the question goes to
  an outside reader.

## Recurring principles

- **Boundaries before mechanisms.** Decide what a part is *allowed* to do
  before deciding *how*; the contract is what others depend on.
- **The simpler shape by default.** Complexity needs positive justification —
  a use case the simpler shape can't serve, not "it might be useful."
- **Stubs are never silent.** A stub is marked, accounted for, and disclosed;
  partial work is reported as partial (constitution, Non-Negotiables).
- **Surface, don't subvert.** Disagreement with a doc or a rule is stated
  explicitly and proposed as a change, never worked around quietly.

## Enforcement (how this stays true without willpower)

1. **The constitution symlink.** `.specify/memory/constitution.md` →
   `hq/00-GENESIS/constitution.md`, so every spec-kit plan is checked against
   GENESIS mechanically.
2. **The structural lint.** `internal/hqlint` is a Go test that rides
   `make test` (locally and in CI): hq layout, research-state legality,
   episode numbering and required fields, index completeness, symlink health,
   and that relative links inside `hq/` resolve.
3. **The skills.** `/research-start`, `/research-graduate`, `/journey-log`
   make the transitions one command each, so the right order is the easy
   order. They stage explicit paths, commit signed, and never push — pushing
   stays a human act.
4. **Orientation.** Root `CLAUDE.md` and `AGENTS.md` point every session
   here first.

## Quality gates (the non-negotiables, in one place)

- Gate: `make fmt && make test && make lint` plus `make compile-deny` — all
  green, nothing skipped, before any "done". (`make test` already runs
  `compile-deny`; it is named separately because the three energy-gradient
  build-tag assertions are the framework's load-bearing invariant and CI runs
  them as their own step.)
- `make compile-deny` asserts that `Publish`, `RequestMany`, and `Conn` are
  structurally absent from `AwarenessContext` — a successful build under any of
  the deny tags is a regression.
- Sign every commit. Never commit `.claude/settings.local.json`.
- Stubs are explicit (constitution) and honest measurement applies to every
  claim — a `[measured]` assertion names the reading behind it.
