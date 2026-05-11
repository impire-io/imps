# Phase 1 Data Model: Harness Core

This document records the entities and relationships the harness operates on. Each entity is described in terms of its fields, the validation rules that apply to it, and (where relevant) its lifecycle. Public-API types are flagged; everything else is internal to the harness package.

The fields described here are the *logical* shape — Go struct definitions in code may differ in field order or unexported helpers. What this document constrains is the contract.

---

## ImpSpec *(public)*

The declarative description of an imp. Constructed by the developer; consumed by the harness at `Run`.

| Field | Type | Required | Notes |
|---|---|---|---|
| `Name` | string | yes | Non-empty. Identifies the imp class (e.g., `complaint-watcher`). |
| `Version` | string | yes | Non-empty. Free-form; recommended semver. |
| `Channels` | `[]ChannelSpec` | no | Zero or more inbound channels (subject or stream). |
| `Awareness` | `AwarenessFn` | yes | The cheap-interpretation callback (FR-009). |
| `Reasoning` | `ReasoningFn` | yes | The expensive-deliberation callback (FR-016). |
| `States` | `[]StateShape` | no | Zero or more local-state-shape declarations. |
| `Actions` | `[]string` | no | The action subject whitelist (declared subjects, pre-resolution). |
| `OnNote` | `func(Entity, any)` | no | Optional Note hook (FR-012). Nil = drop payload after counter increment. |

**Validation rules** (FR-001, FR-002):

- `Name` must be non-empty.
- `Awareness` must be non-nil.
- `Reasoning` must be non-nil.
- Every `StateShape.Name` must be unique within `States` (duplicate → typed `ErrDuplicateStateShape`).
- Every `StateShape.Cap` must be `> 0`.
- Every `Channel.Name` must be unique within `Channels`.
- Duplicate entries in `Actions` are de-duplicated (whitelist semantics are set membership, edge case 13).

Validation runs at `Run` and at `harness.NewImp(spec, ...)` if the latter exists; the error names the offending field (FR-002).

---

## ChannelSpec *(public)*

A declarative description of one inbound subscription.

| Field | Type | Required | Notes |
|---|---|---|---|
| `Name` | string | yes | Identifier used in error messages and observability. Unique within the imp. |
| `Source` | `Source` | yes | One of `SubjectSource` or `StreamSource`. |
| `Decode` | `Decoder` | yes | Returns the typed value used by awareness/reasoning. Errors → record + skip awareness (FR-006). |
| `ExtractEntity` | `EntityExtractor` | yes | Returns the entity for the decoded value. Empty/zero → record + skip awareness (FR-007). |

### Source = SubjectSource

| Field | Type | Required | Notes |
|---|---|---|---|
| `Subject` | string | yes | A NATS subject or pattern (FR-004a). Pre-resolution form (no prefix). |

### Source = StreamSource

| Field | Type | Required | Notes |
|---|---|---|---|
| `Stream` | string | yes | JetStream stream name (FR-004b.i). |
| `FilterSubject` | string | yes | Subject (or pattern within the stream). Pre-resolution form. |
| `Durable` | string | no | When set, names the durable consumer (FR-005a). When empty, ephemeral (FR-005b). |
| `ConsumerConfig` | `ConsumerConfig` | no | Fields passed through to the substrate (ack policy, replay policy, deliver policy, etc.). |

**Validation**:
- Exactly one of `SubjectSource` / `StreamSource` set.
- `SubjectSource.Subject` non-empty.
- `StreamSource.Stream` non-empty.
- `StreamSource.FilterSubject` non-empty.

---

## StateShape *(public)*

A named per-entity state declaration.

| Field | Type | Required | Notes |
|---|---|---|---|
| `Name` | string | yes | Unique within `ImpSpec.States`. |
| `Factory` | `func() any` | yes | Constructs a fresh per-entity instance. Must be safe to call concurrently. |
| `Cap` | int | yes | Maximum number of distinct entities the harness will allocate slots for. Must be `> 0`. |

**Validation**:
- `Name` non-empty and unique within `States`.
- `Factory` non-nil.
- `Cap > 0` (FR-002 — non-positive cap rejected).

**Runtime invariants**:
- First reference to `(Name, entity)` allocates via `Factory()` and indexes it (FR-022).
- Subsequent references return the same instance (FR-023).
- A new-entity allocation when the entity count for `Name` already equals `Cap` returns `ErrCapExceeded{Shape: Name, Count: Cap}` (FR-024). No silent eviction.
- Reads/writes on existing slots succeed regardless of cap state (FR-025).
- References to a state name not declared in `States` return `ErrUnknownStateShape{Shape: name}` (FR-026, edge case "unknown state shape").

---

## AwarenessVerdict *(public)*

Closed sum: one of `Ignore`, `Note(payload any)`, `Wake(reason any, entity Entity)`. Defined as a struct with an unexported discriminator and three exported constructors (see `research.md` R-6).

| Variant | Carries | Harness behavior |
|---|---|---|
| `Ignore` | nothing | No reasoning queued. No note recorded. (FR-011) |
| `Note(payload)` | user-defined `payload` | `OnNote(entity, payload)` invoked if registered. No reasoning queued. (FR-012) |
| `Wake(reason, entity)` | user-defined `reason`, `Entity` | Reasoning queued asynchronously with reason and entity. Channel dispatch returns before reasoning runs. (FR-013) |

**Note**: the `entity` carried by `Wake` may differ from the entity extracted from the message — awareness is allowed to redirect reasoning to a different entity (e.g., a complaint about a customer wakes reasoning on the customer, not the email).

---

## AwarenessContext *(public, interface)*

The typed surface available to the awareness function.

| Method | Return | Notes |
|---|---|---|
| `State(name string, entity Entity) (StateRef, error)` | typed handle to the per-entity state slot | Creates the slot on first reference; `ErrCapExceeded` when shape is full and entity is new; `ErrUnknownStateShape` when name not declared. |

**Critical invariant**: there is **no** `Publish` method on `AwarenessContext`. This is the structural enforcement of FR-014 / FR-029 / SC-006 — calling `awareness.Publish(...)` does not compile because the method does not exist on this interface.

---

## ReasoningContext *(public, interface)*

The typed surface available to the reasoning function.

| Method | Return | Notes |
|---|---|---|
| `State(name string, entity Entity) (StateRef, error)` | same as awareness | Per-entity state access. |
| `Publish(ctx context.Context, subject string, payload []byte) error` | error | Whitelist-checked publish (FR-027). `ErrWhitelistViolation{Subject}` returned for off-whitelist subjects; resolves declared subject through subject-resolver before reaching NATS. |
| `InFlight() int` | current in-flight reasoning count for this imp | Observability surface (FR-021b). |

`ctx` cancellation is wired to harness shutdown — when the harness begins draining, the context passed to in-flight reasoning is cancelled, allowing cooperative cancellation.

---

## StateRef *(public)*

A handle to a per-entity state slot. Concrete type is generic-friendly (`StateRef[T]` with a generic helper) but can also be used as `any`.

| Method | Return | Notes |
|---|---|---|
| `Get() any` | the stored value | Returns the value the factory produced (or the most recent `Set`). |
| `Set(v any) error` | error | Replaces the value. Type checked against the factory's return type — mismatch → typed error. |
| `Update(fn func(any) any) error` | error | Read-modify-write under the slot's lock. |

**Concurrency**: `Update` serializes per slot. Two channels triggering awareness for the same entity concurrently each see a consistent snapshot under `Update` (edge case "Two awareness calls for the same entity arrive concurrently"). Cross-shape ordering is not guaranteed.

---

## ImpIdentity *(public)*

The triple that identifies a running imp instance, queryable from the `Imp` handle (FR-003).

| Field | Type | Notes |
|---|---|---|
| `Name` | string | From `ImpSpec.Name`. |
| `Version` | string | From `ImpSpec.Version`. |
| `SubjectPrefix` | string | The fully-resolved prefix used for channel subscriptions and action publishes (mode-resolved, see Subject Resolution contract). |

---

## NoteRecord *(internal, observable through `OnNote` hook)*

Logical shape of a note delivered to the imp's `OnNote` callback.

| Field | Type | Notes |
|---|---|---|
| `Entity` | `Entity` | The entity from the message that produced the verdict. |
| `Payload` | `any` | The user-defined payload from `Note(payload)`. |

The harness does not retain note records beyond invoking the hook; spec Assumption: "Note records are locally observable but are not emitted to the soulstream or audit stream."

---

## InFlightCounter *(internal, exposed as gauge)*

| Field | Type | Notes |
|---|---|---|
| `count` | `atomic.Int64` | Incremented when reasoning starts, decremented when reasoning returns or panics. Read by `ReasoningContext.InFlight()` and by `Imp.Metrics().InflightReasoning`. |

---

## Metrics *(public, snapshot struct)*

Returned by `Imp.Metrics()`. All counters are non-resetting; snapshots capture point-in-time values.

| Field | Type | Source |
|---|---|---|
| `InflightReasoning` | int64 | gauge — current in-flight reasoning count (FR-021b) |
| `DecodeFailures` | uint64 | counter — increments on FR-006 failure |
| `ExtractionFailures` | uint64 | counter — increments on FR-007 failure |
| `AwarenessPanics` | uint64 | counter — increments on FR-015 recover |
| `ReasoningPanics` | uint64 | counter — increments on FR-021 recover |
| `ReasoningErrors` | uint64 | counter — increments on FR-021 returned error |
| `WhitelistViolations` | uint64 | counter — increments on FR-027 rejection |
| `NotesDelivered` | uint64 | counter — increments per Note verdict (regardless of OnNote registration) |
| `WakesDispatched` | uint64 | counter — increments per Wake verdict |
| `IgnoredVerdicts` | uint64 | counter — increments per Ignore verdict |
| `NakTotal` | uint64 | counter — increments on stream-channel NAK |

---

## ChannelState *(internal)*

Per-channel runtime state held by the harness.

| Field | Type | Notes |
|---|---|---|
| `spec` | `ChannelSpec` | the developer's declaration |
| `resolvedSubject` | string | post-resolution subject (mode-aware) |
| `subscription` | `*nats.Subscription` (subject) or JetStream pull/push handle (stream) | substrate handle |
| `consumerName` | string | for stream channels; empty for ephemeral pre-create |
| `done` | `chan struct{}` | closed on channel shutdown |

---

## RuntimeOptions *(public, applied via functional options)*

| Option | Default | Notes |
|---|---|---|
| `WithDrainWindow(d time.Duration)` | `30 * time.Second` | Applied to graceful shutdown (FR-036). |
| `WithLogger(h slog.Handler)` | `slog.NewTextHandler(io.Discard, …)` | Harness internal logging. |
| `WithPlatformMode(importerAccountPK string)` | non-platform mode | Switches to platform-mode subject resolution (FR-031). |
| `WithSubjectPrefix(prefix string)` | `""` | Required in non-platform mode (FR-030); also applied as the leading segment in platform mode. |

**Validation at Run**:
- Non-platform mode requires non-empty prefix.
- Platform mode requires non-empty `importerAccountPK` (FR-033 — startup fails with a clear configuration error otherwise).

---

## Lifecycle states

The `Imp` handle moves through these states in order. Transitions are one-way.

```
Created → Starting → Running → Draining → Stopped
                  ↘ Failed (terminal, from Starting)
```

| State | Allowed actions |
|---|---|
| Created | `Run` |
| Starting | (no public actions; transient) |
| Running | `Identity`, `Metrics`, `Shutdown` |
| Draining | `Identity`, `Metrics`; new dispatches blocked |
| Stopped | `Identity`, `Metrics` (final snapshot) |
| Failed | `Identity` may be empty; error available via `Run` return |

`Run` blocks until `Shutdown` returns or a fatal error occurs. The handle itself is created at `harness.NewImp(spec, conn, opts...)` and `Run(ctx)` is called separately, allowing the caller to stop via `ctx` cancel as well as `Shutdown`.

---

## Errors *(public, typed)*

| Error | Trigger | Carries |
|---|---|---|
| `ErrSpecInvalid` | construction | offending field name + reason (FR-002) |
| `ErrDuplicateStateShape` | construction | shape name |
| `ErrUnknownStateShape` | runtime State call | shape name |
| `ErrCapExceeded` | runtime State call when full | shape name + current count |
| `ErrWhitelistViolation` | reasoning Publish | offending subject |
| `ErrConfigInvalid` | startup | offending option (e.g., platform-mode without importer pk) |
| `ErrStreamNotFound` | startup, stream channel | stream name |
| `ErrConsumerIncompatible` | startup, stream channel | consumer name + diff summary |
| `ErrSubscriptionFailed` | startup | subject + cause |

All errors satisfy `errors.Is` and `errors.As` for programmatic handling.

---

## Relationships

```
ImpSpec ──has──▶ ChannelSpec[]
              ──has──▶ AwarenessFn
              ──has──▶ ReasoningFn
              ──has──▶ StateShape[]
              ──has──▶ Actions []string  (whitelist)
              ──has──▶ OnNote hook (optional)

Imp (runtime) ──owns──▶ ChannelState[]            (one per ChannelSpec)
              ──owns──▶ StateRegistry             (one per StateShape, capped per shape)
              ──owns──▶ SubjectResolver           (mode-aware)
              ──owns──▶ Whitelist                 (set view of Actions)
              ──owns──▶ InFlightCounter
              ──owns──▶ Metrics counters
              ──holds──▶ *nats.Conn               (caller-supplied)

AwarenessContext ──reads/writes──▶ StateRegistry
ReasoningContext ──reads/writes──▶ StateRegistry
                 ──checks──▶ Whitelist
                 ──publishes via──▶ SubjectResolver + *nats.Conn
```
