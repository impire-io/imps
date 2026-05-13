package imps

import (
	"context"

	"github.com/nats-io/nats.go"
)

// AwarenessContext is the typed surface available to the awareness
// function. It exposes State and Request only — no RequestMany, no
// Publish, no Conn — which is the structural enforcement of the energy
// gradient. An awareness function cannot fan out, cannot fire-and-forget,
// and cannot reach the raw NATS connection. The absences are asserted by
// build-tagged files under integration/compiletest/.
type AwarenessContext interface {
	State(name string, entity Entity) (StateRef, error)

	// Request issues a single NATS request/reply on the declared subject
	// verbatim. Returns the reply payload bytes on success, or one of
	// *ErrNoResponders, *ErrRequestTimeout, a context.Canceled-wrapping
	// error, or a passed-through substrate error. No retry is attempted.
	Request(ctx context.Context, subject string, payload []byte, opts ...RequestOption) ([]byte, error)
}

// ThinkingContext is the typed surface available to the thinking
// function. Publish is a thin convenience over the NATS connection;
// InFlight returns the current in-flight thinking count; Conn returns
// the raw *nats.Conn the harness was constructed with, so generic
// NATS-based clients (e.g., a downstream inference client) can be used
// from thinking without further framework ceremony. Request and
// RequestMany add bounded and fan-out NATS round-trips respectively.
//
// Subject permissioning is the substrate's concern (NATS account ACLs
// on the connection). The harness performs no whitelist check.
type ThinkingContext interface {
	State(name string, entity Entity) (StateRef, error)
	Publish(ctx context.Context, subject string, payload []byte) error
	InFlight() int

	// Conn returns the raw NATS connection the imp was constructed with.
	// Available on ThinkingContext only — awareness has no equivalent
	// method (structural enforcement of the energy gradient). Use this
	// when a downstream NATS-based client takes a *nats.Conn directly.
	Conn() *nats.Conn

	// Request issues a single NATS request/reply on the declared subject
	// verbatim. Same semantics as AwarenessContext.Request.
	Request(ctx context.Context, subject string, payload []byte, opts ...RequestOption) ([]byte, error)

	// RequestMany publishes a single request on the declared subject
	// verbatim and collects every reply received within the effective
	// window (or up to WithRequestManyMax). Returns the collected reply
	// payloads as a slice (possibly empty when no responders replied
	// within the window — that is a legitimate "nobody home" outcome,
	// not an error). The temporary inbox subscription is unsubscribed
	// on every return path.
	RequestMany(ctx context.Context, subject string, payload []byte, opts ...RequestManyOption) ([][]byte, error)
}
