<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan at
specs/006-schedule-channels/plan.md.
<!-- SPECKIT END -->

## How this project is run (read this first)

The SPECKIT block above tracks the active feature; the durable way of working
lives in `hq/`. Before touching anything:

- **`hq/00-GENESIS/` first** — [`vision.md`](hq/00-GENESIS/vision.md),
  [`constitution.md`](hq/00-GENESIS/constitution.md) (the load-bearing
  commitments, non-negotiables, and the anti-drift working agreement, wired into
  spec-kit via the `.specify/memory/constitution.md` symlink), and
  [`how-we-work.md`](hq/00-GENESIS/how-we-work.md). Decisions are held against
  these.
- **[`AGENTS.md`](AGENTS.md)** — the numbered reading order and the
  non-negotiables in brief.
- **The journey duty (required):** every landed feature, concluded research
  investigation, or load-bearing decision gets a numbered episode in
  `hq/04-JOURNEY/` in the same change — `/journey-log` does this (research topics
  get theirs via `/research-graduate`). The structure is enforced by
  `internal/hqlint` under `make test`.
- **Gate before "done":** `make fmt && make test && make lint` plus
  `make compile-deny` — all green, none skipped. Sign every commit; never commit
  `.claude/settings.local.json`.
