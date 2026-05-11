package harness

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// ErrSpecInvalid is returned by NewImp when the supplied ImpSpec fails
// validation. Field names the offending field; Reason describes why.
type ErrSpecInvalid struct {
	Field  string
	Reason string
}

func (e *ErrSpecInvalid) Error() string {
	return fmt.Sprintf("harness: spec invalid: field %q: %s", e.Field, e.Reason)
}

// ErrDuplicateStateShape is returned when two StateShape entries share
// the same Name within ImpSpec.States.
type ErrDuplicateStateShape struct {
	Shape string
}

func (e *ErrDuplicateStateShape) Error() string {
	return fmt.Sprintf("harness: duplicate state shape %q", e.Shape)
}

// ErrConfigInvalid is returned at Run when runtime options fail validation.
// Field names the offending option (e.g., "prefix", "importer_account_pk").
type ErrConfigInvalid struct {
	Field  string
	Reason string
}

func (e *ErrConfigInvalid) Error() string {
	return fmt.Sprintf("harness: config invalid: field %q: %s", e.Field, e.Reason)
}

// ErrStreamNotFound is returned at Run when a StreamSource references a
// JetStream stream that does not exist on the substrate.
type ErrStreamNotFound struct {
	Stream string
}

func (e *ErrStreamNotFound) Error() string {
	return fmt.Sprintf("harness: stream %q not found", e.Stream)
}

// ErrConsumerIncompatible is returned at Run when a declared durable
// consumer exists with a configuration incompatible with the channel's
// declared ConsumerConfig. Diff is a human-readable summary.
type ErrConsumerIncompatible struct {
	Consumer string
	Diff     string
}

func (e *ErrConsumerIncompatible) Error() string {
	return fmt.Sprintf("harness: consumer %q incompatible: %s", e.Consumer, e.Diff)
}

// ErrSubscriptionFailed is returned at Run when establishing a NATS
// subscription fails. Cause wraps the underlying error.
type ErrSubscriptionFailed struct {
	Subject string
	Cause   error
}

func (e *ErrSubscriptionFailed) Error() string {
	return fmt.Sprintf("harness: subscription to %q failed: %v", e.Subject, e.Cause)
}

func (e *ErrSubscriptionFailed) Unwrap() error {
	return e.Cause
}

// ErrNoResponders is returned by Request and (in the publish-refusal
// path) RequestMany when the substrate reports nats.ErrNoResponders for
// the given subject. The "no responders, but window elapsed" outcome of
// RequestMany returns an empty slice instead, not this error.
type ErrNoResponders struct {
	Subject string
}

func (e *ErrNoResponders) Error() string {
	return "harness: no responders for subject " + strconv.Quote(e.Subject)
}

// ErrRequestTimeout is returned by Request when the effective deadline
// elapsed before a reply arrived. It unwraps to context.DeadlineExceeded
// so errors.Is(err, context.DeadlineExceeded) holds.
type ErrRequestTimeout struct {
	Subject string
	Timeout time.Duration
}

func (e *ErrRequestTimeout) Error() string {
	return "harness: request timeout: subject " + strconv.Quote(e.Subject) +
		" exceeded " + e.Timeout.String()
}

func (e *ErrRequestTimeout) Unwrap() error {
	return context.DeadlineExceeded
}
