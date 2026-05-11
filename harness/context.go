package harness

import (
	"context"

	"github.com/nats-io/nats.go"
)

// AwarenessContext is the typed surface available to the awareness
// function. It exposes State only — no Publish, no Conn — which is the
// structural enforcement of the energy gradient. An awareness function
// cannot publish, cannot fan out, and cannot reach the raw NATS
// connection. The absences are asserted by build-tagged files under
// integration/compiletest/.
type AwarenessContext interface {
	State(name string, entity Entity) (StateRef, error)
}

// ReasoningContext is the typed surface available to the reasoning
// function. Publish is a thin convenience over the NATS connection;
// InFlight returns the current in-flight reasoning count; Conn returns
// the raw *nats.Conn the harness was constructed with, so generic
// NATS-based clients (e.g., a downstream inference client) can be used
// from reasoning without further framework ceremony.
//
// Subject permissioning is the substrate's concern (NATS account ACLs
// on the connection). The harness performs no whitelist check.
type ReasoningContext interface {
	State(name string, entity Entity) (StateRef, error)
	Publish(ctx context.Context, subject string, payload []byte) error
	InFlight() int

	// Conn returns the raw NATS connection the imp was constructed with.
	// Available on ReasoningContext only — awareness has no equivalent
	// method (structural enforcement of the energy gradient). Use this
	// when a downstream NATS-based client takes a *nats.Conn directly.
	Conn() *nats.Conn
}
