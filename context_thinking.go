package imps

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
)

// thinkingCtx is the concrete ThinkingContext. Publish, Conn, State,
// InFlight, Request, and RequestMany are all thin shims over the
// harness's runtime state. Subject permissioning is the substrate's
// concern (NATS ACLs); the harness performs no framework-side whitelist.
type thinkingCtx struct {
	registry                 *registry
	conn                     *nats.Conn
	metrics                  *metrics
	logger                   logger
	defaultRequestTimeout    time.Duration
	defaultRequestManyWindow time.Duration
}

func (r *thinkingCtx) State(name string, entity Entity) (StateRef, error) {
	return r.registry.ref(name, entity)
}

func (r *thinkingCtx) Publish(_ context.Context, subject string, payload []byte) error {
	return r.conn.Publish(subject, payload)
}

func (r *thinkingCtx) InFlight() int {
	return int(r.metrics.InflightThinking.Load())
}

func (r *thinkingCtx) Conn() *nats.Conn {
	return r.conn
}

func (r *thinkingCtx) Request(
	ctx context.Context,
	subject string,
	payload []byte,
	opts ...RequestOption,
) ([]byte, error) {
	return requestSingle(ctx, r.conn, r.metrics, r.logger, r.defaultRequestTimeout, subject, payload, opts)
}

func (r *thinkingCtx) RequestMany(
	ctx context.Context,
	subject string,
	payload []byte,
	opts ...RequestManyOption,
) ([][]byte, error) {
	return requestMany(ctx, r.conn, r.metrics, r.logger, r.defaultRequestManyWindow, subject, payload, opts)
}
