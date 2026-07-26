# Phase 0 Research: Soulstream Participation

All material unknowns were resolved before this plan by the
`soulstream-participation` research topic (pre-registered bars, spike,
adversarial pass — [episode 0003](../../hq/04-JOURNEY/0003-soulstream-participation.md))
and its graduated design doc
([`hq/02-DESIGN/0003-soulstream-participation.md`](../../hq/02-DESIGN/0003-soulstream-participation.md)).
This file consolidates those decisions plus the plan-time build questions. No
NEEDS CLARIFICATION markers remain.

## D1. How an imp speaks soulstream: owner library, from a glue module

- **Decision**: consume `github.com/impire-io/soulstream` (`realm` + `topic`
  packages) for every write path; the read path decodes three headers and
  needs no library. The glue lives outside the harness core.
- **Rationale**: the read path was measured at zero implementation (spike);
  the write path (canonicalisation, signing, attribution, mention fan-out) is
  owner-maintained and drifts silently if reimplemented. Harness-core import
  was refuted: it taxes every imp with the coupling.
- **Alternatives considered**: wire reimplementation in imps (refuted —
  silent-drift risk, duplicated maintenance); library import in the core
  module (refuted — constitutional "harness stays small", dependency tax).

## D2. Module shape: nested module with a `replace` on the core

- **Decision**: `soulstream/` directory, module
  `github.com/impire-io/imps/soulstream`, own `go.mod` with
  `replace github.com/impire-io/imps => ../`.
- **Rationale**: the only shape satisfying FR-001 (core `go.mod`
  byte-identical). The `replace` is standard for unpublished multi-module
  repos; it disappears when the core starts tagging releases.
- **Alternatives considered**: separate repository (splits the gate and the
  journey duty; premature while the API settles); core-module package
  (violates FR-001).

## D3. Dependency pin: `github.com/impire-io/soulstream v0.4.0`

- **Decision**: pin the published public tag `v0.4.0` (repo verified PUBLIC;
  tags v0.1.0…v0.4.0 exist). Verify at implementation time that the behaviors
  the spike measured against the local checkout hold at the pinned tag; if
  the local checkout is ahead of v0.4.0 in a way that matters, surface it —
  do not silently depend on unreleased behavior.
- **Rationale**: CI must fetch the dependency without repo-local state; a
  public tag does that. The wire is version 1 and vocabulary evolution is
  additive per the owner's normative protocol doc, so a pin is low-risk.
- **Alternatives considered**: `replace` to a sibling checkout (breaks CI and
  any consumer without the sibling); vendoring (heavier maintenance, no need
  while the repo is public).

## D4. Go toolchain: nested module at `go 1.26.2`, CI via `GOTOOLCHAIN=auto`

- **Decision**: nested `go.mod` declares `go 1.26.2` (floor forced by the
  soulstream dependency). The core stays `go 1.25.0`. CI keeps
  `go-version-file: go.mod` for setup and relies on Go's default
  `GOTOOLCHAIN=auto` to fetch the newer toolchain when building the nested
  module.
- **Rationale**: zero change to the core's toolchain declaration; toolchain
  auto-switching is the supported mechanism for exactly this layout.
- **Alternatives considered**: pointing `go-version-file` at
  `soulstream/go.mod` (works, but couples the core's CI toolchain to the glue
  module's floor); raising the core's `go` directive (violates FR-001).

## D5. Static participation; no runtime join/leave

- **Decision**: topics are declared in `ImpSpec.Channels` at construction;
  the module exposes no runtime add/remove.
- **Rationale**: the protocol has no join/leave — presence is the consumer —
  and no current use case demands a changing topic set. Constitutional
  "simpler shape by default".
- **Alternatives considered**: runtime channel lifecycle in the harness —
  deferred behind the reversal condition registered in episode 0003 (first
  real scenario needing an imp's topic set to change without restart).

## D6. Note bridge semantics: synchronous, typed payload, delegating chain

- **Decision**: `Noted{AnchorOp, Body}` is the bridge's payload type; the
  bridge posts `comment.add` synchronously on the dispatch goroutine
  (best-effort/empty frontier — legal and merge-safe, measured); any other
  payload type is delegated to a wrapped `next` handler (or dropped if nil);
  malformed `Noted` (empty anchor/body) is routed to an error callback, never
  published.
- **Rationale**: notes are the energy gradient's cheap tier — low-rate by
  definition; one publish round-trip on dispatch was measured harmless at
  spike rates. A typed payload keeps the bridge from guessing.
- **Alternatives considered**: async internal queue (named upgrade path if
  note-rate evidence appears; not built speculatively); materialise-first
  posting for clean frontiers (adds a read on the dispatch path — rejected by
  design boundary).

## D7. Decode: headers only, on the dispatch path

- **Decision**: default decoder yields `Op{Type, Author, ID}` from
  `Soulstream-Type`, `Soulstream-Author`, `Nats-Msg-Id`. No payload parse, no
  materialisation. Decoder and entity extractor overridable per channel;
  default entity is the topic path.
- **Rationale**: awareness interprets cheaply (three header reads, measured);
  materialised views are a thinking-tier concern via the owner library.
- **Alternatives considered**: payload-parsing decoder as default (pushes
  JSON work onto every dispatch; available as an override instead); entity =
  author (a reasonable per-persona sharding — available via override, but
  topic-path default matches "the topic is the conversation the imp attends").

## D8. Test strategy: the spike becomes the permanent suite

- **Decision**: integration tests run against embedded NATS
  (`imps/testutil/natstest`) provisioned by the owner's `realm.ProvisionOn`,
  reproducing the research spike end-to-end (baseline-first history → live,
  note round-trip with owner-view verification, attribution refusal, durable
  resume, ephemeral cleanup) plus unit tests per surface. Core byte-identity
  is asserted in the gate (no core file modified; core `go.mod` unchanged).
- **Rationale**: the spike is the measured ground truth of the research; its
  scenarios map 1:1 onto the spec's acceptance scenarios.
- **Alternatives considered**: mocking the soulstream wire (tests the mock,
  not the contract — refuted by "test the contract, not the mechanism").
