# Episode 0002 — The project moves into HQ, and picks a license (2026-07-24)

Not a build — a working-structure decision and a licensing decision, each
recorded with its reversal condition. The framework's process artifacts had
accreted organically: a spec-kit constitution (v2.2.0), three design docs under
`docs/`, and a bare speckit-pointer `CLAUDE.md`. Nothing tied decisions to the
vision mechanically, there was no journal, and the license was "TBD." Decision
[judgment], owner-approved as a wholesale adoption: take the `pra` repository's
**"hq" way of working** in full.

What moved, one home per artifact:

- `docs/00-vision.md` → `hq/00-GENESIS/vision.md` (verbatim).
- The constitution → `hq/00-GENESIS/constitution.md`, amended **v2.2.0 → v2.3.0**:
  a new section, *The Working Agreement (Anti-Drift)* — teach-back as a gate,
  evidence-class tags (`[measured]` / `[mechanism-argument]` / `[judgment]`),
  recorded reversal conditions, and an adversarial pass on framework-identity
  calls — added through the constitution's own amendment discipline (version
  bump + Sync Impact Report). `.specify/memory/constitution.md` is now a
  relative symlink into GENESIS, so every spec-kit plan's Constitution Check
  reads the canonical articles [mechanism-argument].
- `docs/01-anatomy.md` → `hq/02-DESIGN/0001-anatomy.md` and
  `docs/02-capability-service-pattern.md` → `hq/02-DESIGN/0002-…`; a
  gates-not-dates roadmap authored in `03-IMPLEMENTATION/`; this journal
  started with the founding retrospective ([episode 0001](0001-founding-the-harness.md)).

The enforcement is mechanical, not aspirational [mechanism-argument]:
`internal/hqlint` is a Go test that rides `make test` (five areas + READMEs +
GENESIS files + both TEMPLATEs exist; research states legal and non-terminal;
episodes `NNNN-slug.md`, unique, contiguous from 0001, each indexed, each
carrying a `Reversal condition:` line; the constitution symlink resolves into
GENESIS; relative links inside `hq/` resolve) — **verified to fail on a planted
violation, then green with the plant removed** [measured]. The lifecycle
transitions are one-command skills (`/research-start`, `/research-graduate`,
`/journey-log`) that stage explicit paths, commit signed, and never push;
`.github/workflows/ci.yml` runs the same gate plus `compile-deny` on every push
and PR.

**The license is MIT**, the owner's decision [judgment]: the `README` "TBD" is
replaced and a `LICENSE` file added (Copyright 2026 Daan Gerits), mirroring the
sibling `pra` repository. And the two feature specs' `Status` lines were
corrected `Draft` → `Shipped` — bodies left frozen, the spec-vs-code drift
recorded in [episode 0001](0001-founding-the-harness.md).

What it opened: the framework now decides against a fixed point, logs what
happens, and enforces the shape without willpower. Research and the declared-but-
unbuilt milestones (soulstream, sleep/wake, schedule, audit) now have a pipeline
to travel.

Reversal condition: two, recorded now. **Structure** — if, a feature or two from
now, `hq/` lags reality (missing episodes, a stale roadmap, illegal research
states despite the lint), the structure is failing its purpose and we fold back
to the flat layout rather than maintain a facade. **License** — MIT reverses
only by the owner, and only on a distribution constraint MIT cannot meet;
absent that, it stands.

Trail: `hq/` (all five areas), `.specify/memory/constitution.md` (symlink),
`internal/hqlint/`, `.github/workflows/ci.yml`, `LICENSE`, `AGENTS.md`,
`CLAUDE.md`; the `hq-alignment` commit series.
