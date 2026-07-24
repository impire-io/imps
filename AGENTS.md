# Agent guide for the imp framework

Durable instructions for any coding agent working in this repository. The full
rules live in `hq/00-GENESIS/`; this file is the orientation and the
non-negotiables.

## Orientation (read in this order)

1. `hq/00-GENESIS/` — [`vision.md`](hq/00-GENESIS/vision.md) (what an imp is and
   where the framework is pointed), [`constitution.md`](hq/00-GENESIS/constitution.md)
   (the Load-Bearing Commitments, Non-Negotiables, and the anti-drift Working
   Agreement no change may violate), and
   [`how-we-work.md`](hq/00-GENESIS/how-we-work.md) (pipeline, research
   lifecycle, gates). Decisions are held against these.
2. [`hq/04-JOURNEY/README.md`](hq/04-JOURNEY/README.md) — where things stand +
   the episode index: what was built, what was refuted, and why things are the
   way they are.
3. The current feature plan — pointed to by the SPECKIT block in
   [`CLAUDE.md`](CLAUDE.md) (tech stack, structure, commands).
4. [`hq/02-DESIGN/README.md`](hq/02-DESIGN/README.md) — the framework design map
   (anatomy, capability service pattern) with the `[V]`/`[D]`/`[O]` maturity
   legend.

## Non-negotiables (constitution articles, in brief)

- **Quality gate before "done"** (all green, none skipped):
  `make fmt && make test && make lint` plus `make compile-deny`. `make test`
  includes the hq structural lint (`internal/hqlint`); `make compile-deny`
  asserts that `Publish`, `RequestMany`, and `Conn` are structurally absent from
  `AwarenessContext` (a successful build under any deny tag is a regression).
- **Sign commits; never commit `.claude/settings.local.json`.**
- **Imps stay small; capabilities are external.** Default to a capability
  service before adding to the harness. If an imp's purpose needs the word
  "and," it's two imps.
- **The energy gradient is structural, not convention.** Awareness has `State` +
  `Request` only; thinking has the full outbound surface. The boundary is
  compile-enforced, not policy.
- **Imps see one subject path.** Declared subjects are wire subjects verbatim;
  cross-account routing and subject permissioning are substrate concerns (NATS
  account imports and ACLs), never framework code.
- **Stubs are never silent** — marked, accounted for, and disclosed; partial
  work is reported as partial (constitution, Non-Negotiables).

## The flow

- **Research** runs through `/research-start` → investigate →
  `/research-graduate` (`hq/01-RESEARCH/`; never through spec-kit).
- **Features** run the spec-kit flow (`/speckit-specify` → plan → tasks →
  implement) on a numbered branch, and land with the roadmap update, the journey
  episode, and design-doc propagation in the same merge.
- **The journey duty (required):** every landed feature, concluded
  investigation, or load-bearing decision gets a numbered episode in
  `hq/04-JOURNEY/` — `/journey-log` does this (template, index,
  where-things-stand, roadmap).
