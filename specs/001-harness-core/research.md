# Phase 0 Research: Harness Core

This document records the technology and design decisions made before Phase 1 design. Each entry follows: **Decision**, **Rationale**, **Alternatives considered**.

The spec arrived already clarified (see Clarifications session 2026-05-10), so there are no `NEEDS CLARIFICATION` markers to dispatch to research agents. The entries below cover the dependency choices and the design questions a reader needs answered before Phase 1.

---

## R-1: Language and module

**Decision**: Go 1.25, single module rooted at `github.com/impire-io/imps`.

**Rationale**: NATS is the substrate (constitution), and the official Go client (`nats.go`) is the most complete client across core NATS, JetStream, and NATS Micro. The legacy code is also Go 1.25, so contributors share the same toolchain. A single module keeps the developer-facing import surface flat (`harness` is one import path).

**Alternatives considered**:
- Rust + `async-nats` — strong typing helps the awareness/reasoning boundary, but team familiarity and the NATS Micro ecosystem (used by future capability service work) tip the balance back to Go.
- Multi-module repository (separate modules per package) — rejected as overengineering for v1; can be split later if a consumer wants to pin sub-packages independently.

---

## R-2: NATS client library

**Decision**: `github.com/nats-io/nats.go` for both core NATS subscriptions and JetStream consumers.

**Rationale**: It is the upstream client; JetStream (`jetstream.New(nc)`) and core (`nc.Subscribe`) both come from the same connection. Pinning to one client avoids version skew between subscription types.

**Alternatives considered**:
- `nats.ws` — explicitly forbidden by repository CLAUDE.md ("do NOT use nats.ws. It is deprecated. Use github.com/nats-io/nats.js instead"); not applicable here as the harness is Go-server-side, not browser.

---

## R-3: NATS context loading (examples + tests)

**Decision**: `github.com/synadia-io/orbit.go/natscontext` for any code that resolves a connection from a named context (examples, end-to-end tests against developer-local NATS).

**Rationale**: Repository CLAUDE.md mandates this package for context-based connection in Go code. The harness library itself takes a `*nats.Conn` from the caller, so it does not depend on `natscontext` directly — only the example and the optional CLI that wraps it do.

**Alternatives considered**:
- Reimplementing context parsing inside the harness — rejected; orbit.go is the project standard and avoids a parallel implementation.
- Hard-coding URLs in examples — rejected; examples should demonstrate the same connection idiom production imps use.

---

## R-4: Embedded NATS server for tests

**Decision**: `github.com/nats-io/nats-server/v2` started in-process per test or per integration package, with JetStream enabled when a test needs stream channels. Helper lives in `testutil/natstest`.

**Rationale**: Spec Independent-Test sections all describe driving an embedded server. In-process avoids docker dependency in CI and lets tests bind to ephemeral ports without coordination. JetStream is enabled per-test by passing a temporary store directory; cleanup is `t.Cleanup`-driven.

**Alternatives considered**:
- A long-lived shared server across the test binary — rejected; isolation between tests becomes fragile (consumer/durable state leaks). Per-test cost is small (~100 ms startup) and acceptable given test counts.
- Mocking the NATS interface — rejected; the spec explicitly tests against a real substrate (FR-005, FR-008a, stream channel acceptance scenarios).

---

## R-5: Logging

**Decision**: Standard library `log/slog` (Go 1.25 built-in). Harness exposes a `slog.Handler` option on construction; default is a no-op handler.

**Rationale**: The constitution penalizes adding to the harness; pulling in zerolog or zap is unnecessary when slog is in the standard library and structured. Letting the imp author inject a handler keeps the harness's logging policy out of their way.

**Alternatives considered**:
- `github.com/rs/zerolog` (used in legacy) — heavier and predates slog; no benefit for v1.
- No logging from the harness at all — rejected; some operator-relevant lifecycle events (start, shutdown, ack failure) genuinely benefit from a log line.

---

## R-6: Awareness verdict representation in Go

**Decision**: A closed `Verdict` value implemented as a struct with an unexported discriminator and three exported constructors (`Ignore()`, `Note(payload any)`, `Wake(reason any, entity Entity)`). Awareness functions return `Verdict` (not `(Verdict, error)`); a panic from awareness is recovered by the harness (FR-015).

**Rationale**: Go does not have sum types. Three options were considered (interface + type-switch, sealed struct + variant tag, multiple return values). A sealed struct with constructors:
- prevents user code from constructing invalid verdicts (the discriminator is unexported);
- lets the harness pattern-match via a single `verdict.kind` switch without runtime type assertions;
- keeps the awareness signature `func(...) Verdict` clean (no error channel).

**Alternatives considered**:
- `interface { isVerdict() }` with three exported types `Ignore{}`, `Note{Payload any}`, `Wake{Reason any, Entity Entity}` — symmetrical and discoverable but lets a user define their own type with the unexported method (impossible across packages, fine in practice). Decision is to revisit if user code wants to construct a verdict in a way the constructor doesn't allow; for v1 the constructor approach wins on simplicity.
- Separate awareness return type (e.g., `(reason any, entity Entity, ok bool)`) — collapses three states to a flag and loses the `Note` payload.

---

## R-7: Awareness vs. reasoning context — typed surface

**Decision**: Two distinct Go interfaces:

```go
type AwarenessContext interface {
    State(name string, entity Entity) (StateRef, error)
}

type ReasoningContext interface {
    State(name string, entity Entity) (StateRef, error)
    Publish(ctx context.Context, subject string, payload []byte) error
    InFlight() int   // observability — count of currently-running reasoning invocations on this imp
}
```

`Publish` is **not** present on `AwarenessContext`. A developer calling `awareness.Publish(...)` gets a compile error because the method does not exist on the awareness type — this satisfies FR-014 and SC-006 structurally.

**Rationale**: Two separate interfaces is the smallest surface that delivers the structural boundary the constitution requires. Putting both methods on a single interface and gating `Publish` at runtime is rejected by the spec's User Story 3 acceptance scenario 1 ("the code fails to compile").

**Alternatives considered**:
- One context type with a runtime gate — rejected; collapses the energy gradient from a structural property to a policy check.
- Three contexts (awareness, reasoning, lifecycle) — premature; v1 doesn't need a lifecycle context.

---

## R-8: Per-entity state store

**Decision**: A typed registry keyed by state-shape name to a per-shape `entityMap` (a `sync.Map` of `Entity → *stateBox`). Each `entityMap` carries the shape's per-entity cap; allocation increments an atomic counter and returns `ErrCapExceeded` when the counter would exceed the cap. Reads after cap-reached succeed (FR-025).

**Rationale**: `sync.Map` is the right shape for the access pattern (read-heavy on existing keys, occasional first-write that allocates). The cap counter is enforced separately so the cap check is atomic with allocation: a CAS-style increment that backs out on cap-exceeded.

**Alternatives considered**:
- `map[Entity]T` + `sync.RWMutex` per shape — simpler but contention on `Lock()` for the first write of every new entity is real under SC-011 workloads.
- A bounded LRU — explicitly rejected by the spec ("MUST NOT silently evict any existing entity", FR-024).
- Generic `StateRef[T]` via Go generics — kept as an option for the public surface but the internal store erases the type to `any` to avoid per-shape goroutine pools.

---

## R-9: Reasoning concurrency

**Decision**: Each `Wake` verdict launches a fresh goroutine with the reasoning function. The harness tracks in-flight count via `sync.WaitGroup` plus an atomic int64 (`atomic.Int64`) for the observability surface (FR-021b). No bound on the count (FR-021a).

**Rationale**: A goroutine-per-wake matches Go's idiom and keeps the per-message dispatch overhead bounded (SC-010). The atomic counter is read for observability without locking the WaitGroup.

**Alternatives considered**:
- Worker pool with bounded queue — rejected; spec defers bounded-concurrency policies to a later feature and the developer owns wake-rate × reasoning-latency budget in v1.
- Per-entity goroutine — rejected; would require per-entity scheduling state and contradicts FR-019 ("Reasoning invocations for the same entity MAY run concurrently").

---

## R-10: Stream channel ack timing and consumer lifecycle

**Decision**:
- Ack the substrate message after awareness returns a verdict (any of `Ignore` / `Note` / `Wake`) — this is the spec's clarified Q2.
- NAK on decode failure, extraction failure, awareness panic, or awareness error.
- For a declared durable name `D` on stream `S`: bind to `D` if it exists with a config compatible with the channel's declared config; create `D` with the declared config if absent; fail startup with a clear error if `D` exists but is incompatible (FR-005a).
- Ephemeral consumers (no durable name) are created at startup and torn down on clean shutdown (FR-005b).

**Rationale**: Acking at awareness completion is what the clarification settled (spec Clarifications Q2). It honors the energy gradient — reasoning latency does not block stream redelivery — while keeping decode/extraction/awareness failures observable through the substrate's max-deliveries policy (FR-008a, edge case "max-deliveries exceeded").

**Compatibility check** for an existing durable consumer: compare the channel's declared filter subject and (if specified) ack policy / replay policy / deliver policy against the live consumer info; mismatch → startup error (FR-005a, edge case "durable consumer config incompatible").

**Alternatives considered**:
- Ack after reasoning completes — rejected by clarification Q2; would couple stream redelivery to reasoning latency.
- Always-create (ignore existing durable) — rejected; loses replay position across restarts (User Story 4 acceptance scenario 1).

---

## R-11: No subject transformation

**Decision**: The framework performs no subject transformation. Channel subscriptions bind on the declared subject verbatim; `Publish` publishes on the declared subject verbatim. No `WithSubjectPrefix` option, no resolver, no `<prefix>.<declared>` mapping. Cross-account routing and tenant scoping are configured at the substrate via NATS account imports.

**Rationale**: Per the constitution's "Imps see one subject path" principle (v2.2.0), the subjects an imp declares are the substrate subjects on the wire. The earlier `<prefix>.<declared>` rule produced unpredictable behavior — "I configured X but the imp publishes on `whatever.X`" — and was redundant with NATS account-level scoping that an operator already has to configure for multi-tenant deployments. Removing the framework-side prefix collapses the matrix.

**Historical note**: Earlier drafts of this feature carried (a) a `platform_mode` flag + `importer_account_pk` segment, then (b) a single-form `<prefix>.<declared>` resolver after constitution v2.1.0. Constitution v2.2.0 retired the prefix entirely; the `WithSubjectPrefix` option, the internal `resolver`, and `ImpIdentity.SubjectPrefix` were all removed in the same cleanup.

**Alternatives considered**:
- Mode-specific code paths in `dispatch/` and `stream/` — rejected then and now.
- Configuring two prefixes (one for channels, one for actions) — rejected; channels and actions used the same convention while the resolver existed, and now both pass through verbatim.
- Letting the framework keep an optional prefix that defaults to empty — rejected; a per-deployment knob the imp's source doesn't see produces exactly the "I configured X" surprise the v2.2.0 amendment was motivated by.

---

## R-12: Subject permissioning is the substrate's concern

**Decision**: No framework-side action whitelist. `Publish` is a thin shim over `nc.Publish` — no pre-check, no `ErrWhitelistViolation`, no `Actions` field on `ImpSpec`. Subject permissioning is configured at NATS at the account / connection level (account ACLs).

**Rationale**: The earlier `Actions` whitelist was defense-in-depth — a runtime check the framework did because it didn't trust the imp's code to behave. But the framework runs the imp's code; if the imp can't be trusted, a framework-side check doesn't help (the imp could call `nc.Publish` directly, or — with `ReasoningContext.Conn()` added in the same cleanup — could pull the raw connection out anyway). NATS ACLs are an out-of-process check that actually constrains a compromised process. Defense-in-depth that doesn't depend on the controlled component being trustworthy is the right shape.

**Historical note**: Earlier drafts had `ImpSpec.Actions []string`, a runtime whitelist check in `Publish`, an `ErrWhitelistViolation` typed error, and a `WhitelistViolations` metric. All retired in the constitution-v2.2.0 cleanup.

**Alternatives considered**:
- Keeping the whitelist as documentation only (no enforcement) — rejected; documentation that doesn't fail at use-site rots.
- Whitelist + NATS ACLs (defense in depth) — rejected; the framework-side check duplicates substrate behavior without adding meaningful protection (see Rationale).

---

## R-13: Note records and observability surface

**Decision**: A configurable hook on the imp spec — `OnNote func(entity Entity, payload any)` — receives the payload from a `Note` verdict (FR-012). If the imp author does not register a hook, `Note` payloads are dropped after recording the verdict. Counters exposed alongside:
- `inflight_reasoning` (gauge, `atomic.Int64`, satisfies FR-021b)
- `decode_failures`, `extraction_failures`, `awareness_panics`, `awareness_errors`, `reasoning_panics`, `reasoning_errors`, `nak_total` (counters, `atomic.Uint64`)

These are read via `Imp.Metrics()` returning a struct snapshot. No Prometheus or external metrics dependency in v1.

**Rationale**: Spec's Note semantics are "locally observable" (Assumption "Note records are locally observable") — soulstream emission belongs to the soulstream feature. A hook covers the v1 case without baking a transport into the harness. Counters via atomics satisfy SC-010's "growth less than linear" with negligible overhead.

**Alternatives considered**:
- A buffered channel of Note records — rejected; introduces backpressure on awareness if the consumer doesn't drain.
- OpenTelemetry metrics dependency — rejected as added weight against the "harness is small" commitment; an OTEL adapter can be a separate package later.

---

## R-14: Lifecycle and shutdown drain

**Decision**: Two-phase shutdown:
1. Stop accepting new messages — cancel subscriptions and the JetStream consumer pull loops; return ephemeral-consumer cleanup to the substrate.
2. Wait up to `drain_window` (default `30 s`, configurable per spec via `harness.WithDrainWindow`) for in-flight reasoning to complete via `WaitGroup.Wait` against a context with the drain deadline.
3. Return regardless once the deadline elapses; the harness does not cancel in-flight reasoning's user code (Go does not allow that without cooperative cancellation, which the reasoning context can opt into via the standard `context.Context`).

The reasoning context's `context.Context` is cancelled at step 1, so reasoning that respects ctx-cancel exits promptly; reasoning that ignores it runs until completion or the drain deadline.

**Rationale**: Matches FR-036 and User Story 8 acceptance scenario 3. The default drain window is 30 s — enough for typical reasoning, short enough to surface stuck reasoning quickly. The combined "cancel ctx + wait for WG" pattern is idiomatic Go.

**Alternatives considered**:
- Force-killing reasoning goroutines — Go does not support this; rejected.
- No drain window, immediate return — would orphan goroutines and leak whatever the reasoning was holding (DB handles, half-published actions). Rejected.

---

## R-15: Decode and entity extractor signatures

**Decision**:

```go
type Decoder[T any] func(msg Message) (T, error)
type EntityExtractor[T any] func(decoded T) (Entity, error)
```

`Entity` is a typed string (`type Entity string`); empty string is invalid per FR-007. `Message` carries the raw NATS payload, the resolved subject, headers, and (for stream channels) the underlying ack/nak handles abstracted by the harness so the extractor and decoder cannot ack directly.

**Rationale**: Generic decode/extract types let an imp work in typed messages from awareness onward. Hiding the ack handle behind the harness abstraction means user code can't accidentally short-circuit FR-008a's ack timing.

**Alternatives considered**:
- Untyped `any` decoded values — rejected; loses the spec's "awareness sees structured data, not raw bytes" promise (`docs/01-harness-anatomy.md`).
- Single combined `func(msg) (Entity, T, error)` — flattened, but two functions read more clearly and let the entity extractor be reused across decoders.

---

## R-16: Project layout and visibility

**Decision**: Public surface lives in one package (`harness`); implementation lives under `internal/`. Tests live in a top-level `integration/` package (black-box) plus per-package unit tests as needed.

**Rationale**: Documented in `plan.md` Structure Decision. The constitution's "harness is small" commitment is most visibly honored when the developer-facing import is a single package.

**Alternatives considered**:
- Splitting public types across multiple packages (`harness/spec`, `harness/runtime`) — rejected; an imp author should be able to read one godoc page and have the full API.
- Mirroring the legacy `imps/`, `runtime/`, `core/`, `coordination/` split — explicitly rejected by the spec ("the new awareness/reasoning split does not map onto the legacy projection/derivation/reactor split").

---

## R-17: Build, lint, format, test workflow

**Decision**: Top-level `Makefile` with `fmt`, `tidy`, `test` (`go test -race -count=1 ./...`), `lint` (`golangci-lint run ./...`), `build`, and `check` (composes the others). `.golangci.yml` enables `errcheck`, `govet`, `staticcheck`, `unused`, `gofmt`, `goimports`, `revive`.

**Rationale**: Mirrors legacy `Makefile` so contributors have the same muscle memory; the global CLAUDE.md mandates linting after every change and `make fmt && make test && make lint` as the reality-checkpoint command.

**Alternatives considered**:
- Just (`justfile`) or Mage — no benefit over the existing Makefile idiom.

---

## Open questions deferred to subsequent features

The spec already lists `FR-NS-1`…`FR-NS-4` as out-of-scope. Recording them here for completeness — none block this feature:

- Capability clients (inference, knowledge, tools) and the bounded-capability surface in awareness.
- Soulstream channels and the `Note → soulstream` emission.
- KV channels (NATS Key-Value bucket watchers as inbound message sources).
- Schedule channels via NATS server-side scheduling.
- Sleep/wake snapshot integration and the wake-hook.
- Persistence/rehydration of memory across hard restarts.
- Audit-record emission to a tenant-scoped audit stream.
- Bounded-concurrency policy on reasoning (cap + overflow).
- Per-entity reasoning serialization opt-in.
- Queue-group semantics for subject channels and harness-imposed sharding for multi-replica deployments.
- Wildcard membership in the action whitelist.
