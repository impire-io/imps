package imps

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
)

// reasoningCtx is the concrete ReasoningContext. Publish, Conn, State,
// InFlight, Request, and RequestMany are all thin shims over the
// harness's runtime state. Subject permissioning is the substrate's
// concern (NATS ACLs); the harness performs no framework-side whitelist.
type reasoningCtx struct {
	registry                 *registry
	conn                     *nats.Conn
	metrics                  *metrics
	logger                   logger
	defaultRequestTimeout    time.Duration
	defaultRequestManyWindow time.Duration
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

func (r *reasoningCtx) Request(
	ctx context.Context,
	subject string,
	payload []byte,
	opts ...RequestOption,
) ([]byte, error) {
	return requestSingle(ctx, r.conn, r.metrics, r.logger, r.defaultRequestTimeout, subject, payload, opts)
}

func (r *reasoningCtx) RequestMany(
	ctx context.Context,
	subject string,
	payload []byte,
	opts ...RequestManyOption,
) ([][]byte, error) {
	return requestMany(ctx, r.conn, r.metrics, r.logger, r.defaultRequestManyWindow, subject, payload, opts)
}
