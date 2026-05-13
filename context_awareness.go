package imps

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
)

// awarenessCtx is the concrete AwarenessContext. It exposes State and
// Request — no Publish, no RequestMany, no Conn — which is the
// structural enforcement of the energy gradient.
type awarenessCtx struct {
	registry              *registry
	conn                  *nats.Conn
	metrics               *metrics
	logger                logger
	defaultRequestTimeout time.Duration
}

func (a *awarenessCtx) State(name string, entity Entity) (StateRef, error) {
	return a.registry.ref(name, entity)
}

func (a *awarenessCtx) Request(
	ctx context.Context,
	subject string,
	payload []byte,
	opts ...RequestOption,
) ([]byte, error) {
	return requestSingle(ctx, a.conn, a.metrics, a.logger, a.defaultRequestTimeout, subject, payload, opts)
}
