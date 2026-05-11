package harness

import (
	"context"

	"github.com/nats-io/nats.go"
)

// reasoningCtx is the concrete ReasoningContext. Publish, Conn, State,
// and InFlight are all thin shims over the harness's runtime state.
// Subject permissioning is the substrate's concern (NATS ACLs); the
// harness performs no framework-side whitelist.
type reasoningCtx struct {
	registry *registry
	conn     *nats.Conn
	metrics  *metrics
	logger   logger
}

func (r *reasoningCtx) State(name string, entity Entity) (StateRef, error) {
	return r.registry.ref(name, entity)
}

func (r *reasoningCtx) Publish(_ context.Context, subject string, payload []byte) error {
	return r.conn.Publish(subject, payload)
}

func (r *reasoningCtx) InFlight() int {
	return int(r.metrics.InflightReasoning.Load())
}

func (r *reasoningCtx) Conn() *nats.Conn {
	return r.conn
}
