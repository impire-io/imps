# Implementation Plan: Soulstream Participation

**Branch**: `004-soulstream-participation` | **Date**: 2026-07-25 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/004-soulstream-participation/spec.md`
**Design source**: [`hq/02-DESIGN/0003-soulstream-participation.md`](../../hq/02-DESIGN/0003-soulstream-participation.md) (graduated research, [episode 0003](../../hq/04-JOURNEY/0003-soulstream-participation.md))

## Summary

Ship soulstream topic participation as a **nested Go module**
(`soulstream/`, module `github.com/impire-io/imps/soulstream`) beside the
harness core, which stays byte-identical. Three surfaces: `TopicChannel`
builds a standard `ChannelSpec` on the existing `StreamSource` (stream
`SOULSTREAM`, filter `SOULSTREAM.TOPICS.OPS.<path>`, deliver-all default,
header-only decode); `NoteBridge` turns the existing `Note` verdict carrying a
`Noted{AnchorOp, Body}` payload into `comment.add` contributions posted via
the owner's library; `Participant` wraps the imp's own NATS connection in
`realm.NewClient` (persona for writes, optional Ed25519 signer, never closes
the conn). The research spike (episode 0003) becomes the permanent
integration suite, running against embedded NATS provisioned by the owner's
`realm.ProvisionOn`. The Makefile and CI extend to cover the nested module.

## Technical Context

**Language/Version**: Go — core module stays `go 1.25.0` (untouched); nested module `go 1.26.2` (floor set by the soulstream dependency); CI relies on `GOTOOLCHAIN=auto` for the nested module
**Primary Dependencies**: `github.com/impire-io/imps` (the core, via `replace => ../`), `github.com/impire-io/soulstream v0.4.0` (`realm` + `topic` packages only — measured transitive additions: `google/uuid`, `gowebpki/jcs`, `synadia-io/orbit.go/natscontext`; the MCP SDK is not pulled), `github.com/nats-io/nats.go v1.52.0`
**Storage**: N/A — all substrate state (op-log stream, notify stream) is owned and provisioned by the soulstream project; this module reads/writes it as a client
**Testing**: `go test -race` with embedded NATS (`imps/testutil/natstest`) + `realm.ProvisionOn` for a real realm shape; the end-to-end suite reproduces the research spike
**Target Platform**: any Go platform reaching a NATS substrate with JetStream
**Project Type**: library (second, opt-in module in the existing single-repo layout)
**Performance Goals**: dispatch path unchanged from the harness baseline (decode = three header reads, no allocation-heavy parsing, no materialisation); note bridge = one JetStream publish round-trip per note, synchronous by design
**Constraints**: core `go.mod` byte-identical; no core source change; `make compile-deny` green; no new channel kind; static topic set (fixed at `Run`); no membership/rollup/ordering duties
**Scale/Scope**: ~5 source files + tests in the nested module; topics-per-imp is small (units, not thousands — each is one JetStream consumer)

## Constitution Check

*GATE: evaluated against `hq/00-GENESIS/constitution.md` v2.3.0 (via the `.specify/memory/constitution.md` symlink). Re-checked after Phase 1 — no drift.*

| Article | Verdict | Evidence |
|---|---|---|
| Imps stay small and agile | PASS | Nothing enters the harness; the module is opt-in. An imp that doesn't participate links none of this. Core `go.mod` byte-identity is FR-001 and a test-time assertion. |
| The energy gradient is structural | PASS | Awareness's surface is unchanged; the `Note` verdict (shipped) is the only awareness-side emission, and the publish happens in the `OnNote` hook outside awareness's hands. `compile-deny` stays in the gate. Research measured the whole loop with zero harness changes. |
| Capabilities are external; the harness is small | PASS | The module lives beside the core, not in it. No capability is added to the harness. |
| Coordination happens through the soulstream | PASS | This feature is that commitment made real: topics as channels, notes and turns as contributions. |
| Wire protocols are per-capability; deployment uniform | PASS | The soulstream repo owns its wire protocol; this module consumes it through the owner's client and defines none of it. |
| Non-negotiable: awareness does not call unbounded capabilities | PASS | Awareness gains no call. The bridge publish is bounded (one op) and sits in the note hook, which was already the imp author's code path. |
| Non-negotiable: no direct provider/SDK calls in imp code | **JUSTIFIED** | The rule targets capability access (LLM SDKs, DB drivers) bypassing capability subjects. `github.com/impire-io/soulstream` is the **coordination-medium client** — the same category as `nats.go`, which every imp already links. The soulstream is a load-bearing commitment ("Coordination happens through the soulstream"), not a capability service; reimplementing its wire (canonicalisation, signing) was adversarially refuted in episode 0003 (silent-drift risk). Recorded in Complexity Tracking. |
| Non-negotiable: no central registry | PASS | No discovery, no registry. Startup errors on a missing stream come from the harness's existing named errors. |
| Imps see one subject path | PASS | `TopicChannel` passes the literal OPS subject verbatim; no rewriting. |
| Boundaries before mechanisms / simpler shape | PASS | Static participation only; runtime join/leave deferred with a registered reversal condition (episode 0003). |
| Stubs are explicit, never silent | PASS | No stubs planned; every surface in this plan ships tested or is cut from scope openly. |

## Project Structure

### Documentation (this feature)

```text
specs/004-soulstream-participation/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: consolidated decisions (all pre-resolved by design 0003)
├── data-model.md        # Phase 1: entities and validation
├── quickstart.md        # Phase 1: end-to-end usage walkthrough
├── contracts/
│   ├── go-api.md        # Phase 1: exported surface contract of the nested module
│   └── repo-gate.md     # Phase 1: Makefile/CI coverage contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (passed)
└── tasks.md             # Phase 2 output (/speckit-tasks — not created by /speckit-plan)
```

### Source Code (repository root)

```text
soulstream/                  # NEW nested module: github.com/impire-io/imps/soulstream
├── go.mod                   # go 1.26.2; requires imps (replace => ../), soulstream v0.4.0, nats.go
├── go.sum
├── doc.go                   # package overview: participation model, energy-gradient placement
├── op.go                    # Op type + header-only decode function
├── channel.go               # TopicChannel(path, ...TopicChannelOption) → imps.ChannelSpec
├── participant.go           # Participant: NewParticipant(nc, realm, persona, ...), Topic(path), StartTopic(...)
├── notebridge.go            # Noted payload type + NoteBridge(participant, next) → OnNote func
├── channel_test.go          # unit: spec construction, option application, decode/extract defaults
├── notebridge_test.go       # unit: payload routing (Noted vs other), error paths
├── participant_test.go      # unit: config validation, conn ownership (Close never called)
└── participation_test.go    # integration: the research spike as permanent suite (embedded NATS + ProvisionOn)

Makefile                     # EDITED: tidy/test/lint/build extended to the nested module
.github/workflows/ci.yml     # EDITED: build/test/lint steps cover the nested module
```

**Structure Decision**: a nested module (own `go.mod`) rather than a package in
the core module — the only shape that satisfies FR-001 (core `go.mod`
byte-identical) while letting the glue consume the owner's library. The core
module is referenced by a `replace github.com/impire-io/imps => ../` directive
until the core publishes tagged versions; the soulstream dependency pins the
published public tag `v0.4.0`.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Nested module imports `github.com/impire-io/soulstream` (reads as "SDK in imp code") | The soulstream is the constitution's coordination medium, and its wire (RFC-8785 canonicalisation, Ed25519 signing, attribution guard) must be produced exactly as the owner defines it | Reimplementing the wire in this repo duplicates owner-maintained code and drifts silently (a wrong canonicalisation still publishes, but nobody can verify the signature); putting the import in the core module taxes every imp with the coupling — both refuted in the episode-0003 adversarial pass |
| Second Go module in the repo (build surface grows) | Only shape that keeps the core `go.mod` byte-identical (FR-001, constitutional "harness stays small") while the glue links the owner's library | A single-module repo either pollutes the core `go.mod` (rejected above) or forbids the dependency entirely, which forces the wire reimplementation (rejected above) |
