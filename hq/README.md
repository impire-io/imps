# HQ — the framework's headquarters

Everything about *how this project is run* lives here. Code lives at the module
root (the `imps` package) with black-box tests under `integration/`, and frozen
per-feature spec-kit artifacts in `specs/NNN-*/`. Everything else — why the
framework exists, what we're investigating, what we've designed, what we're
building, and what happened — lives in one of the five areas below.

| Area | What it holds | When you touch it |
|---|---|---|
| [`00-GENESIS/`](00-GENESIS/README.md) | Vision, constitution, working rules | When deciding *whether* / *how* to do something |
| [`01-RESEARCH/`](01-RESEARCH/README.md) | Active research topics (one folder each) | While investigating an open question |
| [`02-DESIGN/`](02-DESIGN/README.md) | Architecture & framework designs, evidence trails | When research graduates, or a build changes behavior |
| [`03-IMPLEMENTATION/`](03-IMPLEMENTATION/README.md) | The roadmap: what to build, in what order, behind which gates | When planning or landing a feature |
| [`04-JOURNEY/`](04-JOURNEY/README.md) | Numbered episodes: the honest log of what happened | Whenever a feature lands, research concludes, or a load-bearing decision is made |

## The pipeline

```
01-RESEARCH ──graduates──▶ 02-DESIGN ──/speckit-specify──▶ specs/NNN + code
     │                         ▲                                │
     │ (abandoned)             │ (behavioral changes            │
     │                         │  propagate back)               │
     ▼                         │                                ▼
04-JOURNEY ◀──────── every ending writes an episode ◀── 03-IMPLEMENTATION
                                                          (roadmap updated)
```

- Research topics live in `01-RESEARCH/<slug>/` and end in exactly one of
  three states: **graduated to design**, **graduated to artifact**, or
  **abandoned**. Every ending produces a numbered episode in `04-JOURNEY/`;
  the topic folder is then removed (git history keeps the full trail).
- Designs in `02-DESIGN/` are written functional-level and explicit enough to
  hand to `/speckit-specify`. Implementation always goes through the spec-kit
  flow; research never does.
- The lifecycle transitions are mechanized as skills: `/research-start`,
  `/research-graduate`, `/journey-log`. The structure itself is enforced by
  `internal/hqlint`, a Go test that rides `make test` (locally and in CI).

**If in doubt** — about whether to build something, how to decide, or whether
a shortcut is acceptable — the answer is in [`00-GENESIS/`](00-GENESIS/README.md).
Hold the decision against `vision.md` and `constitution.md`; if it still isn't
clear, that's a teach-back conversation, not a judgment call to make alone.
