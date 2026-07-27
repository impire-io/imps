# Implementation Plan: Sleep, Wake, and Per-Entity Persistence

**Branch**: `005-sleep-wake-persistence` | **Date**: 2026-07-26 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/005-sleep-wake-persistence/spec.md`
**Design source**: [`hq/02-DESIGN/0004-sleep-wake-persistence.md`](../../hq/02-DESIGN/0004-sleep-wake-persistence.md) (graduated research, [episode 0005](../../hq/04-JOURNEY/0005-sleep-wake-persistence.md))

## Summary

Ship the durable tier of the two-tier memory boundary as **`imps/persist`**,
a plain package in the existing core module (no new dependencies — the
reference backend uses `jetstream`, already required). `Store[T]` is
bounded (LRU, default 256), **write-through** (the update return is the
durability contract; the snapshot is continuous), **rehydrate-on-access**
(lazy restore), with a per-entity wake hook fired exactly once per
rehydration before the state is observable. `Beacon` is the imp-level sleep
clock (stamp / slept-for) for the pre-`Run` wake gate. `Backend` is the
minimal boundary (Get/Put/Delete + `ErrNotFound`) with `KVBackend` over
JetStream KV as reference implementation only. The research spike (episode
0005) becomes the permanent test suite. The root harness package and the
core `go.mod` stay byte-identical.

## Technical Context

**Language/Version**: Go 1.25.0 — the core module, unchanged; `persist` is a package inside it (generics over `T`)
**Primary Dependencies**: none added. `github.com/nats-io/nats.go` (jetstream KV for the reference backend) and the embedded test server are already core dependencies
**Storage**: behind the `Backend` boundary; reference implementation JetStream KV; bucket provisioning is the operator's act
**Testing**: `go test -race` in the core module (the package rides the existing gate untouched); embedded NATS via `testutil/natstest`; same-package tests may inject the store's clock for deterministic elapsed assertions; the restart round-trip runs a real imp
**Target Platform**: any Go platform; backend reachability is the only requirement
**Project Type**: library package (core module)
**Performance Goals**: one backend round-trip per `Update` (write-through), at most one per rehydrating `Get`; store operations serialize under one mutex (correctness first; per-entity locking is the named upgrade if contention evidence appears)
**Constraints**: root package untouched; core `go.mod`/`go.sum` byte-identical; no registry interaction; wake exactly-once under concurrency; eviction never touches the backend; no enumeration/janitor duties
**Scale/Scope**: ~6 source files + tests; resident bound default 256 ("the default stays small"); entity count bounded only by the backend

## Constitution Check

*GATE: evaluated against `hq/00-GENESIS/constitution.md` v2.3.0. Re-checked after Phase 1 — no drift.*

| Article | Verdict | Evidence |
|---|---|---|
| Imps stay small and agile | PASS | Opt-in package; default bound 256 residents; lazy restore keeps cold starts small. Nothing enters the root package. |
| The energy gradient is structural | PASS | Awareness's surface is unchanged; store calls from awareness are bounded round-trips (the `Request` discipline, M1 note-bridge precedent, recorded `[mechanism-argument]` in episode 0005). `compile-deny` stays in the gate. |
| Capabilities are external; the harness is small | PASS | The store is not a capability service — it is local memory's durable tier, per the anatomy's Memory section. The backend is behind a minimal boundary; no service, no protocol. |
| Externalize state, internalize specialization | PASS | Cross-imp shared state remains forbidden; the store is per-imp, per-store-name keyspace. FR-012 forbids enumeration/scanning (no shadow registry). |
| Non-negotiable: imps do not share local memory | PASS | Keyspace is per store name; sharing a bucket across imps is operator keyspace discipline, and the package neither enables nor mediates cross-imp reads. Documented. |
| Non-negotiable: no direct provider/SDK calls in imp code | PASS | The reference backend speaks the substrate (NATS JetStream KV) — the same category as `nats.go` itself, per the M1/episode-0005 reasoning. No new SDK enters any module. |
| Boundaries before mechanisms | PASS | The `Backend` interface and envelope contract are the design; JetStream KV is reference only; nothing requires it. |
| The simpler shape by default | PASS | Package not module (no deps to fence); one mutex not per-entity locks; write-through not write-back; lazy restore not startup replay — each with its named upgrade path. |
| Stubs are explicit, never silent | PASS | Backend failures error, never zero (FR-004/SC-006); no stubs planned. |
| Sleep is the common case | PASS | This feature is that commitment made real: write-through means stopping is safe by construction; the Beacon supplies the wake reading. |

## Project Structure

### Documentation (this feature)

```text
specs/005-sleep-wake-persistence/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: consolidated decisions (pre-resolved by design 0004)
├── data-model.md        # Phase 1: entities and validation
├── quickstart.md        # Phase 1: end-to-end usage walkthrough
├── contracts/
│   ├── go-api.md        # Phase 1: exported surface contract of the package
│   └── repo-gate.md     # Phase 1: gate coverage + byte-identity contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (passed)
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
persist/                     # NEW package in the core module: github.com/impire-io/imps/persist
├── doc.go                   # package overview: two-tier rule, wake contract, awareness discipline
├── backend.go               # Backend interface, ErrNotFound, KVBackend (jetstream.KeyValue adapter)
├── codec.go                 # Codec[T], JSONCodec[T] default
├── store.go                 # Store[T], NewStore, options, Get/Update/Delete/Resident, envelope, LRU
├── beacon.go                # Beacon: NewBeacon, Stamp, SleptFor
├── backend_test.go          # unit: KVBackend mapping (ErrNotFound), failing-backend stub behavior
├── store_test.go            # unit: bound/eviction/no-loss, wake exactly-once with injected clock,
│                            #   zero-value path, error surfacing, concurrency (-race)
├── beacon_test.go           # unit: first-start absence, stamp→slept-for measurement
└── restart_test.go          # integration: the research spike productized — real imp, stop,
                             #   fresh instance, codec-equal rehydration + wake elapsed
```

No Makefile or CI change is needed: `persist` lives in the core module, so
`go build ./...`, `go test -race ./...`, and `golangci-lint run ./...`
already cover it.

**Structure Decision**: a package in the core module (not a nested module) —
the design doc's explicit refinement: M1's module boundary fenced a new
dependency; `persist` adds none, so the simpler shape wins. Byte-identity of
the root package and `go.mod` is asserted at the gate (contracts/repo-gate.md).

## Complexity Tracking

> No constitution violations to justify. The one structural addition (a
> second memory surface beside `ImpSpec.States`) carries its registered
> reversal condition from episode 0005: real two-tier inconsistency bugs or
> measured dispatch-latency damage moves the boundary into the harness.
