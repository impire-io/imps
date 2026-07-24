# 00-GENESIS — why this framework exists and how it decides

This folder is the fixed point every decision is held against. It changes
rarely, deliberately, and always with a journey episode recording why.

| File | Role |
|---|---|
| [`vision.md`](vision.md) | What an imp is, who the framework is for, what it deliberately refuses to become |
| [`constitution.md`](constitution.md) | The load-bearing commitments, working principles, non-negotiables, and the anti-drift working agreement — the rules no work may violate. Canonical copy — spec-kit's Constitution Check reads it through the `.specify/memory/constitution.md` symlink |
| [`how-we-work.md`](how-we-work.md) | The process: pipeline state machine, research lifecycle, quality gates, documentation duties, and the working agreement in practice |

## The decision test

When a choice comes up — a new direction, a shortcut, a scope change — run it
through, in order:

1. **Vision**: does it serve what `vision.md` says an imp is for? If it serves
   something else (impressiveness, convenience, a generalist imp, a bigger
   harness), say so out loud. When in tension, the small-and-agile constraint
   wins.
2. **Constitution**: does it violate a Load-Bearing Commitment or a
   Non-Negotiable? These don't bend for feature work; if one genuinely must
   change, that's an amendment with a version bump, a Sync Impact Report, and a
   journey episode — never a quiet exception.
3. **Working agreement**: if the decision is load-bearing, it does not get
   recorded until it survives teach-back, carries its evidence class
   (`[measured]` / `[mechanism-argument]` / `[judgment]`), names its reversal
   condition, and — for framework-identity calls — has had the other side
   argued at full strength. See `how-we-work.md`.

If the test doesn't produce a clear answer, the decision waits for the human.
