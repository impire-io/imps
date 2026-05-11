# Contract: Public API

The harness's developer-facing surface. This is what an imp author imports. Every type, function, and method here is part of the contract — implementation can change, the contract holds.

Package: `github.com/impire-io/imps/harness`

---

## Construction

```go
// NewImp validates the spec and returns a non-running handle.
// The caller supplies the NATS connection (the harness does not connect itself).
// Options are applied left-to-right; later options override earlier ones.
//
// Returns ErrSpecInvalid (with field name) on validation failure.
// Returns ErrConfigInvalid on option validation failure.
func NewImp(spec ImpSpec, nc *nats.Conn, opts ...Option) (*Imp, error)
```

```go
type Option func(*runtimeOptions)

func WithDrainWindow(d time.Duration) Option        // default 30 s
func WithLogger(h slog.Handler) Option              // default discard
func WithSubjectPrefix(prefix string) Option        // required in non-platform mode
func WithPlatformMode(importerAccountPK string) Option  // switches to platform-mode resolution
```

Defaults:
- `WithDrainWindow` → `30 * time.Second`
- `WithLogger` → discard handler
- non-platform mode unless `WithPlatformMode` is supplied
- empty prefix unless `WithSubjectPrefix` is supplied

Startup config validation (at `Run`, not at `NewImp`):
- non-platform mode + empty prefix → `ErrConfigInvalid`
- platform mode + empty importer account public key → `ErrConfigInvalid` (FR-033)

---

## Lifecycle

```go
// Run establishes every declared subscription, registers identity, then dispatches
// messages until ctx is cancelled or Shutdown is called. Run blocks.
//
// On startup failure, Run returns immediately with no subscriptions registered
// and no dispatch goroutines started (FR-035).
//
// On clean shutdown, Run returns nil. On drain timeout, Run returns
// errors.Join(context.DeadlineExceeded, ...) — the imp is stopped, but some
// reasoning may have been left in flight beyond the deadline.
func (i *Imp) Run(ctx context.Context) error

// Shutdown initiates graceful shutdown:
//   1. Stop accepting new messages (cancel subscriptions, JetStream pulls).
//   2. Cancel the context passed to in-flight reasoning.
//   3. Wait up to drain_window for in-flight reasoning to complete.
//   4. Return no later than drain_window + small ε after the call.
//
// Calling Shutdown more than once is safe (subsequent calls return nil).
// Shutdown can be called from any goroutine.
func (i *Imp) Shutdown(ctx context.Context) error
```

```go
// Identity returns the running imp's name, version, and resolved subject prefix.
// Available during Running, Draining, and Stopped states (FR-003).
func (i *Imp) Identity() ImpIdentity

type ImpIdentity struct {
    Name          string
    Version       string
    SubjectPrefix string
}
```

```go
// Metrics returns a non-resetting snapshot of harness counters.
// Safe to call concurrently with dispatch.
func (i *Imp) Metrics() Metrics

type Metrics struct {
    InflightReasoning   int64
    DecodeFailures      uint64
    ExtractionFailures  uint64
    AwarenessPanics     uint64
    ReasoningPanics     uint64
    ReasoningErrors     uint64
    WhitelistViolations uint64
    NotesDelivered      uint64
    WakesDispatched     uint64
    IgnoredVerdicts     uint64
    NakTotal            uint64
}
```

---

## ImpSpec

```go
type ImpSpec struct {
    Name      string
    Version   string
    Channels  []ChannelSpec
    Awareness AwarenessFn
    Reasoning ReasoningFn
    States    []StateShape
    Actions   []string                      // declared subjects, pre-resolution
    OnNote    func(entity Entity, payload any)  // optional
}
```

```go
type Entity string  // empty Entity ("") is invalid
```

---

## Channels

```go
type ChannelSpec struct {
    Name          string
    Source        Source                // SubjectSource OR StreamSource (sealed)
    Decode        Decoder
    ExtractEntity EntityExtractor
}

// Source is a sealed interface. Two implementations:
type Source interface{ isSource() }

type SubjectSource struct {
    Subject string  // pre-resolution subject or pattern
}
func (SubjectSource) isSource() {}

type StreamSource struct {
    Stream         string
    FilterSubject  string  // pre-resolution
    Durable        string  // empty = ephemeral consumer
    ConsumerConfig ConsumerConfig
}
func (StreamSource) isSource() {}

// ConsumerConfig is a passthrough to JetStream consumer config fields the harness
// supports (ack policy, replay policy, deliver policy, max-deliveries, etc.).
// Documented in detail in the stream-channel contract.
type ConsumerConfig struct { /* fields */ }
```

```go
// Decoder turns a raw inbound message into the typed value awareness will see.
// Errors are recorded; awareness is not invoked for that message (FR-006).
type Decoder func(msg Message) (any, error)

// EntityExtractor returns the entity for a decoded value. Empty entity is
// treated as a failure; awareness is not invoked (FR-007).
type EntityExtractor func(decoded any) (Entity, error)

// Message is the harness's view of an inbound substrate message.
// Subject is the resolved subject the message arrived on. Headers and Data
// are passthroughs from NATS. Ack/NAK is owned by the harness (not exposed
// here) so user code cannot short-circuit FR-008a's ack timing.
type Message struct {
    Subject string
    Reply   string
    Headers nats.Header
    Data    []byte
}
```

---

## Awareness

```go
type AwarenessFn func(
    ctx context.Context,
    decoded any,
    entity Entity,
    awareness AwarenessContext,
) Verdict

type AwarenessContext interface {
    State(name string, entity Entity) (StateRef, error)
    // No Publish method. Compile-time enforcement of FR-014.
}
```

The `ctx` passed to awareness is the per-message dispatch context — short-lived, cancelled when the message has been acked. Awareness MUST NOT spawn goroutines that outlive the call.

---

## Reasoning

```go
type ReasoningFn func(
    ctx context.Context,
    reason any,
    entity Entity,
    reasoning ReasoningContext,
) error

type ReasoningContext interface {
    State(name string, entity Entity) (StateRef, error)
    Publish(ctx context.Context, subject string, payload []byte) error
    InFlight() int  // FR-021b — current in-flight reasoning count for this imp
}
```

The `ctx` passed to reasoning is the harness shutdown context — cancelled when shutdown begins, allowing cooperative cancellation. Reasoning that does not respect ctx-cancel runs until completion or the drain window elapses.

A returned `error` from reasoning is recorded against the `ReasoningErrors` counter; no retry is attempted (FR-NS-3).

---

## State

```go
type StateShape struct {
    Name    string
    Factory func() any
    Cap     int  // > 0
}

type StateRef interface {
    Get() any
    Set(v any) error
    Update(fn func(any) any) error  // serialized per slot
}
```

Errors returned by `AwarenessContext.State` and `ReasoningContext.State`:
- `ErrUnknownStateShape{Shape: name}` — name not declared in spec.
- `ErrCapExceeded{Shape: name, Count: cap}` — shape full and entity is new.

After cap is reached, further `State(name, existingEntity)` calls succeed (FR-025).

---

## Verdict

```go
// Verdict is a closed sum: Ignore | Note(payload) | Wake(reason, entity).
// Construct only via Ignore(), Note(), Wake() — the kind discriminator is unexported.
type Verdict struct {
    // unexported fields
}

func Ignore() Verdict
func Note(payload any) Verdict
func Wake(reason any, entity Entity) Verdict
```

The harness pattern-matches verdicts internally; user code does not need to inspect them.

---

## Errors

All errors are exported sentinel types satisfying `errors.Is`/`errors.As`.

```go
type ErrSpecInvalid struct{ Field, Reason string }
type ErrDuplicateStateShape struct{ Shape string }
type ErrUnknownStateShape struct{ Shape string }
type ErrCapExceeded struct{ Shape string; Count int }
type ErrWhitelistViolation struct{ Subject string }
type ErrConfigInvalid struct{ Field, Reason string }
type ErrStreamNotFound struct{ Stream string }
type ErrConsumerIncompatible struct{ Consumer, Diff string }
type ErrSubscriptionFailed struct{ Subject string; Cause error }
```

Each error's `Error()` message names the offending value (FR-002, FR-005a, FR-005c, FR-024, FR-026, FR-027, FR-033).

---

## Compile-time guarantees (verified by tests)

The following must hold at compile time, not at runtime:

1. `var _ = func(a AwarenessContext) { a.Publish(...) }` MUST fail to compile.
   The integration suite includes a build-tagged file under `integration/compiletest/` whose presence-of-build-failure is the assertion (SC-006).

2. `Verdict` MUST not be constructible with a value that is neither Ignore, Note, nor Wake. A test in the verdict package attempts to forge one and confirms the discriminator is unexported.

3. `Source` MUST not be implementable outside the `harness` package. The `isSource()` method is unexported.
