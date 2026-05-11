package harness

import (
	"context"

	"github.com/nats-io/nats.go"
)

// reasoningCtx is the concrete ReasoningContext. Publish is whitelist-
// checked and subject-resolved before reaching NATS; off-whitelist publishes
// return *ErrWhitelistViolation without touching the substrate.
type reasoningCtx struct {
	registry  *registry
	resolver  *resolver
	whitelist *whitelist
	conn      *nats.Conn
	metrics   *metrics
	logger    logger
}

func (r *reasoningCtx) State(name string, entity Entity) (StateRef, error) {
	return r.registry.ref(name, entity)
}

func (r *reasoningCtx) Publish(_ context.Context, subject string, payload []byte) error {
	if err := r.whitelist.check(subject); err != nil {
		r.metrics.WhitelistViolations.Add(1)
		r.logger.warn("whitelist violation", "subject", subject)
		return err
	}
	resolved := r.resolver.resolve(subject)
	return r.conn.Publish(resolved, payload)
}

func (r *reasoningCtx) InFlight() int {
	return int(r.metrics.InflightReasoning.Load())
}
