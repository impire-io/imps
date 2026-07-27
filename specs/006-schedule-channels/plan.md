# Implementation Plan: Schedule Channels

**Branch**: `006-schedule-channels` | **Date**: 2026-07-27 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/006-schedule-channels/spec.md`
**Design source**: [`hq/02-DESIGN/0005-schedule-channels.md`](../../hq/02-DESIGN/0005-schedule-channels.md) (graduated research, [episode 0008](../../hq/04-JOURNEY/0008-schedule-channels.md))

## Summary

Ship M3 as **`imps/schedule`**, a plain package in the core module (zero new
dependencies, root package untouched, no Makefile/CI wiring — the `persist`
shape). `Channel(stream, target, opts…)` is sugar over the **existing**
`StreamSource` with a header-only `Tick{Subject, Scheduler, Next}` decoder
and the target subject as entity; `Register`/`Deregister` are typed builders
over the substrate's six scheduling headers, for the thinking/operator tier.
The framework runs no timers, produces no ticks, and owns no schedule
registry — the server owns the clock (verified in the pinned
`nats-server v2.14.0`, episode 0008). The research spike becomes the
permanent integration suite.

## Technical Context

**Language/Version**: Go 1.25.0 — the core module, unchanged
**Primary Dependencies**: none added (`nats.go/jetstream` already required exposes `AllowMsgSchedules`/`AllowMsgTTL` and header publishing)
**Storage**: the operator-provisioned stream holding schedules and ticks; the package never provisions
**Testing**: `go test -race` in the core module; embedded NATS via `testutil/natstest`; the spike productized (warm/cold/TTL with counts) plus header round-trip tests reading stored schedules back
**Target Platform**: any Go platform reaching a JetStream substrate ≥ the pinned server's scheduling support
**Project Type**: library package (core module)
**Performance Goals**: zero steady-state cost in the package (no goroutines, no timers); registration is one publish; consumption is the existing channel path
**Constraints**: root package untouched; `go.mod` byte-identical; no Makefile/CI diff; no tick filtering; no schedule state in the package; registration unreachable from awareness's bounded surface
**Scale/Scope**: ~4 source files + tests; cadences seconds-or-slower at imp scale

## Constitution Check

*GATE: evaluated against `hq/00-GENESIS/constitution.md` v2.3.0. Re-checked after Phase 1 — no drift.*

| Article | Verdict | Evidence |
|---|---|---|
| Imps stay small and agile | PASS | Opt-in package; nothing enters the harness; an imp that doesn't schedule links nothing new. |
| The energy gradient is structural | PASS | Consumption rides existing channels; registration needs a `jetstream` handle awareness structurally lacks (`compile-deny` unchanged); the package documentation forbids handing it one (M1/M2a discipline). |
| Capabilities are external; the harness is small | PASS | Tick production is the substrate's, not a framework capability; the package is typed headers + channel sugar. |
| Sleep is the common case | PASS | Ticks fire warm or cold by construction; TTL-pruned catch-up is what a woken imp replays (M2b-orthogonal). |
| Non-negotiable: no central registry | PASS | No schedule registry, reconciler, or janitor — one schedule per subject is the server's semantics, surfaced. |
| Imps see one subject path | PASS | Target subjects pass verbatim; no rewriting. |
| Boundaries before mechanisms / simpler shape | PASS | Thin package over documentation-only argued in episode 0008 (typed headers remove a real error class); everything else deferred to the substrate. |
| Stubs are explicit, never silent | PASS | No stubs planned; fail-fast validation on empty pattern/target. |

## Project Structure

### Documentation (this feature)

```text
specs/006-schedule-channels/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: consolidated decisions (pre-resolved by design 0005)
├── data-model.md        # Phase 1: entities and validation
├── quickstart.md        # Phase 1: end-to-end usage walkthrough
├── contracts/
│   ├── go-api.md        # Phase 1: exported surface contract
│   └── repo-gate.md     # Phase 1: gate coverage + byte-identity contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (passed)
└── tasks.md             # Phase 2 output
```

### Source Code (repository root)

```text
schedule/                    # NEW package in the core module: github.com/impire-io/imps/schedule
├── doc.go                   # package overview: server owns the clock, tiers, TTL consequence
├── tick.go                  # Tick type, header-only decoder, default entity, default name
├── channel.go               # Channel(stream, target, ...ChannelOption) → imps.ChannelSpec
├── register.go              # Register/Deregister + RegisterOption header builders
├── channel_test.go          # unit: spec construction, options, header-only decode
├── register_test.go         # integration: header round-trip on stored schedule, replace, deregister, fail-fast validation
└── schedule_test.go         # integration: the research spike productized (warm/cold/TTL with counts, provenance)
```

No Makefile or CI change: the core-module gate covers the package.

**Structure Decision**: a package in the core module (the `persist`
precedent — no dependencies to fence, so no module boundary). Byte-identity
asserted at the gate per contracts/repo-gate.md.

## Complexity Tracking

> No constitution violations to justify. The single judgment call — thin
> package over documentation-only — carries its episode-0008 reversal
> condition (a substrate header re-shaping flips typed headers from safety
> to liability; the integration suite reads it out).
