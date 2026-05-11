package harness

import "context"

// AwarenessContext is the typed surface available to the awareness
// function. It exposes State only — there is no Publish method, which is
// the structural enforcement of the energy gradient. Calling
// awareness.Publish(...) does not compile.
type AwarenessContext interface {
	State(name string, entity Entity) (StateRef, error)
}

// ReasoningContext is the typed surface available to the reasoning
// function. Publish is whitelist-checked before reaching NATS; off-whitelist
// subjects return ErrWhitelistViolation. InFlight returns the current
// in-flight reasoning count for the imp (observability surface).
type ReasoningContext interface {
	State(name string, entity Entity) (StateRef, error)
	Publish(ctx context.Context, subject string, payload []byte) error
	InFlight() int
}

// TODO(test): a build-tagged file under integration/compiletest/ asserts
// at compile time that AwarenessContext does not expose Publish (SC-006).
