# Episode 0001 — Founding the harness: the substrate, then the sweep (2026-05-10 – 2026-05-13)

The framework's founding arc, recorded in retrospect so the journey starts with
an honest baseline. On 2026-05-10 the project adopted GitHub Spec Kit, a
constitution (v2.0.0), and three framework design docs — vision, anatomy, and
the capability-service-pattern. Two features then shipped through the spec-kit
flow:

- **`001-harness-core`** (PR #1): the in-process Go substrate — channels
  (core-subject and JetStream stream), awareness dispatch with the
  three-verdict return (`Ignore` / `Note` / `Think`), thinking invocation in its
  own goroutine, per-entity local memory, and action publishing. The
  awareness/thinking boundary is **compile-enforced** — build-tagged files under
  `integration/compiletest/` assert that `Publish`, `RequestMany`, and `Conn` do
  *not* exist on `AwarenessContext`, and `make compile-deny` requires each to
  fail to compile [measured].
- **`002-capability-client`** (PR #2): the outbound NATS surface — `Request`,
  `RequestMany`, `Publish`, and `Conn` — byte-shaped, literal subjects, no
  framework codec and no retry policy.

Then the sweep. Three follow-on PRs changed the shape of the shipped code
without changing what it does [measured, git history]: the package was
**flattened to the module root** (the import is now `github.com/impire-io/imps`,
not `.../imps/harness`) and **`Wake` → `Think`** (PR #3), user-facing docs and
the README were swept to match (PR #4), and **`Reasoning` → `Thinking`** for
verdict/layer consistency (PR #5). Separately, the constitution's v2.2.0
amendment and its coordinated code cleanup **removed the action whitelist**
(`ImpSpec.Actions`, `*ErrWhitelistViolation`, the `WhitelistViolations` metric)
and the `WithSubjectPrefix` option: subject permissioning became a substrate
concern (NATS account ACLs), and the energy-gradient boundary is now purely the
type-level absence proven by `compile-deny` [mechanism-argument] — ACLs are the
right layer for subject permissioning, and "which methods exist on each context
type" is a stronger boundary than a runtime whitelist because it cannot be
reached at all.

**The honest drift, recorded here rather than papered over.** The frozen spec
bodies (`specs/001-harness-core/`, `specs/002-capability-client/`) describe the
*pre-sweep* code: the `harness/` package path, the `Wake`/`Reasoning` names, and
the action whitelist. Spec bodies are point-in-time artifacts and are **not
rewritten** — that would forge the record. Instead: their `Status` lines were
corrected `Draft` → `Shipped` (both features did ship), the anatomy design doc
was propagated to match the shipped code (whitelist gone, boundary
compile-enforced; now `hq/02-DESIGN/0001-anatomy.md`), and this episode is the
canonical note that spec prose and code have diverged where the sweep touched
them — the code and the design docs are the truth, the specs are the frozen
argument that produced them [judgment].

What it opened: the substrate is at a natural pause. The parts of an imp still
unbuilt — soulstream coordination channels, sleep/wake + snapshot persistence,
schedule channels, audit emission — are tagged `[D]` in the anatomy and
sequenced in [`../03-IMPLEMENTATION/roadmap.md`](../03-IMPLEMENTATION/roadmap.md).

Reversal condition: the one embedded direction decision is the whitelist
removal. It reverses only on evidence that substrate ACLs are insufficient — an
imp observed reaching a subject that NATS account ACLs cannot constrain in a
real deployment — in which case a framework-side boundary returns as an
explicit, opt-in, specced check, never a silent whitelist. The rest of this
episode records completed builds.

Trail: [`../00-GENESIS/vision.md`](../00-GENESIS/vision.md),
[`../02-DESIGN/0001-anatomy.md`](../02-DESIGN/0001-anatomy.md),
[`../02-DESIGN/0002-capability-service-pattern.md`](../02-DESIGN/0002-capability-service-pattern.md);
`specs/001-harness-core/`, `specs/002-capability-client/`; commits `424943b`
(constitution v2.0.0), `0f09eaa` (spec-kit bootstrap), `af5ea0d` (design docs),
`94f4458`→`3c605af` (#1), `6c775db`→`3d68bc1` (#2), `bedf5fb`→`15bc037` (#3
flatten + Wake→Think), `3b415c8`→`21f67d6` (#4 docs sweep),
`0874332`→`ebd5cb7` (#5 Reasoning→Thinking).
