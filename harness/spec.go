package harness

import "context"

// Entity identifies the subject (in the linguistic sense) the imp's
// awareness/reasoning operate on for a given message. The empty Entity ("")
// is invalid; entity extractors that return "" cause the message to be
// recorded as an extraction failure and skipped.
type Entity string

// AwarenessFn is the cheap-interpretation callback. It is invoked
// synchronously on the dispatch goroutine; it must not block on slow work.
// A panic is recovered by the harness and recorded as AwarenessPanics.
type AwarenessFn func(
	ctx context.Context,
	decoded any,
	entity Entity,
	awareness AwarenessContext,
) Verdict

// ReasoningFn is the expensive-deliberation callback. It is invoked on a
// fresh goroutine per Wake verdict. The ctx passed is the harness shutdown
// context — cancelled when shutdown begins so reasoning can cooperatively
// exit. A returned error is recorded against ReasoningErrors.
type ReasoningFn func(
	ctx context.Context,
	reason any,
	entity Entity,
	reasoning ReasoningContext,
) error

// StateShape declares one named per-entity state slot. Factory MUST be
// safe to call concurrently. Cap MUST be > 0 and bounds the maximum number
// of distinct entities for which the harness will allocate slots.
type StateShape struct {
	Name    string
	Factory func() any
	Cap     int
}

// ImpSpec is the declarative description of an imp. It is constructed by
// the developer and consumed by the harness at NewImp / Run.
//
// Validation runs at NewImp; the error returned names the offending field.
//
// Outbound subject permissioning is the substrate's concern (NATS account
// ACLs on the connection), not the framework's. The spec does not declare
// an outbound subject whitelist.
type ImpSpec struct {
	Name      string
	Version   string
	Channels  []ChannelSpec
	Awareness AwarenessFn
	Reasoning ReasoningFn
	States    []StateShape
	OnNote    func(entity Entity, payload any)
}

// ImpIdentity identifies a running imp instance.
type ImpIdentity struct {
	Name    string
	Version string
}

// Metrics is a non-resetting snapshot of harness counters. All values are
// monotonic across the lifetime of the imp except InflightReasoning, which
// is a gauge.
type Metrics struct {
	InflightReasoning  int64
	DecodeFailures     uint64
	ExtractionFailures uint64
	AwarenessPanics    uint64
	ReasoningPanics    uint64
	ReasoningErrors    uint64
	NotesDelivered     uint64
	WakesDispatched    uint64
	IgnoredVerdicts    uint64
	NakTotal           uint64

	// RequestCalls is the total number of Request invocations (success +
	// failure) across awareness and reasoning. Calls made via Conn() bypass
	// this counter.
	RequestCalls uint64
	// RequestManyCalls is the total number of RequestMany invocations.
	// Calls made via Conn() bypass this counter.
	RequestManyCalls uint64
	// RequestNoResponders is the number of Request (and publish-refused
	// RequestMany) invocations that returned *ErrNoResponders.
	RequestNoResponders uint64
	// RequestTimeouts is the number of Request invocations that returned
	// *ErrRequestTimeout.
	RequestTimeouts uint64
}
