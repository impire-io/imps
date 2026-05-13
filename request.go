package imps

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
)

// natsStatusHdr is the NATS protocol "Status" header used by the server
// to signal protocol-level conditions (e.g., "503 No Responders Available
// For Request") to subscribers on a reply inbox. The literal is duplicated
// from nats.go's unexported statusHdr constant.
const natsStatusHdr = "Status"

// RequestOption configures a single Request invocation. Pass instances
// returned by WithRequestTimeout to AwarenessContext.Request or
// ThinkingContext.Request.
type RequestOption func(*requestOptions)

// RequestManyOption configures a single RequestMany invocation. Pass
// instances returned by WithRequestManyWindow / WithRequestManyMax to
// ThinkingContext.RequestMany.
type RequestManyOption func(*requestManyOptions)

type requestOptions struct {
	timeout time.Duration
}

type requestManyOptions struct {
	window time.Duration
	max    int
}

// WithRequestTimeout overrides the harness's default request timeout for
// a single Request invocation. A non-positive duration is a silent no-op
// — the harness default applies.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(o *requestOptions) {
		if d > 0 {
			o.timeout = d
		}
	}
}

// WithRequestManyWindow overrides the harness's default RequestMany
// collection window for a single invocation. A non-positive duration is
// a silent no-op — the harness default applies.
func WithRequestManyWindow(d time.Duration) RequestManyOption {
	return func(o *requestManyOptions) {
		if d > 0 {
			o.window = d
		}
	}
}

// WithRequestManyMax caps a single RequestMany invocation at n replies;
// the call returns as soon as n replies have arrived without waiting for
// the rest of the window. A non-positive n means "no cap; collect for
// the full window".
func WithRequestManyMax(n int) RequestManyOption {
	return func(o *requestManyOptions) {
		if n > 0 {
			o.max = n
		}
	}
}

// requestSingle dispatches a single NATS request/reply on the literal
// subject. Increments RequestCalls before any work; on failure paths,
// increments the matching failure-mode counter. Translates substrate
// errors into the framework's typed categories.
func requestSingle(
	ctx context.Context,
	nc *nats.Conn,
	m *metrics,
	log logger,
	defaultTimeout time.Duration,
	subject string,
	payload []byte,
	opts []RequestOption,
) ([]byte, error) {
	m.RequestCalls.Add(1)

	cfg := requestOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}
	effective := cfg.timeout
	if effective <= 0 {
		effective = defaultTimeout
	}

	derived, cancel := context.WithTimeout(ctx, effective)
	defer cancel()

	start := time.Now()
	msg, err := nc.RequestWithContext(derived, subject, payload)
	elapsed := time.Since(start)

	if err == nil {
		log.debug("request",
			"subject", subject,
			"bytes", len(msg.Data),
			"elapsed", elapsed,
			"outcome", "ok",
		)
		return msg.Data, nil
	}

	switch {
	case errors.Is(err, nats.ErrNoResponders):
		m.RequestNoResponders.Add(1)
		out := &ErrNoResponders{Subject: subject}
		log.warn("request failed",
			"subject", subject,
			"category", "no_responders",
			"cause", err,
		)
		return nil, out
	case errors.Is(ctx.Err(), context.Canceled),
		errors.Is(err, context.Canceled):
		log.warn("request failed",
			"subject", subject,
			"category", "canceled",
			"cause", err,
		)
		return nil, err
	case errors.Is(err, context.DeadlineExceeded):
		// The caller's ctx already had a deadline that beat ours: surface
		// it verbatim. Otherwise it was our derived deadline: produce
		// *ErrRequestTimeout.
		if cerr := ctx.Err(); errors.Is(cerr, context.DeadlineExceeded) {
			log.warn("request failed",
				"subject", subject,
				"category", "caller_deadline",
				"cause", err,
			)
			return nil, err
		}
		m.RequestTimeouts.Add(1)
		out := &ErrRequestTimeout{Subject: subject, Timeout: effective}
		log.warn("request failed",
			"subject", subject,
			"category", "timeout",
			"cause", err,
		)
		return nil, out
	default:
		log.warn("request failed",
			"subject", subject,
			"category", "substrate",
			"cause", err,
		)
		return nil, err
	}
}

// requestMany dispatches a single request and collects every reply that
// arrives within the effective window, optionally capped at WithRequestManyMax.
// The temporary inbox subscription is unsubscribed on every return path.
func requestMany(
	ctx context.Context,
	nc *nats.Conn,
	m *metrics,
	log logger,
	defaultWindow time.Duration,
	subject string,
	payload []byte,
	opts []RequestManyOption,
) ([][]byte, error) {
	m.RequestManyCalls.Add(1)

	cfg := requestManyOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}
	effective := cfg.window
	if effective <= 0 {
		effective = defaultWindow
	}
	maxReplies := cfg.max
	bufSize := maxReplies
	if bufSize < 64 {
		bufSize = 64
	}

	inbox := nc.NewRespInbox()
	replyCh := make(chan *nats.Msg, bufSize)
	sub, err := nc.ChanSubscribe(inbox, replyCh)
	if err != nil {
		log.warn("request_many failed",
			"subject", subject,
			"category", "subscribe",
			"cause", err,
		)
		return nil, err
	}
	defer func() {
		if uerr := sub.Unsubscribe(); uerr != nil {
			log.warn("request_many unsubscribe failed",
				"subject", subject,
				"err", uerr,
			)
		}
	}()

	if err := nc.PublishMsg(&nats.Msg{
		Subject: subject,
		Reply:   inbox,
		Data:    payload,
	}); err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			m.RequestNoResponders.Add(1)
			log.warn("request_many failed",
				"subject", subject,
				"category", "no_responders",
				"cause", err,
			)
			return nil, &ErrNoResponders{Subject: subject}
		}
		log.warn("request_many failed",
			"subject", subject,
			"category", "publish",
			"cause", err,
		)
		return nil, err
	}

	deadline := time.NewTimer(effective)
	defer deadline.Stop()

	start := time.Now()
	collected := make([][]byte, 0, bufSize)

	for {
		select {
		case msg, ok := <-replyCh:
			if !ok {
				log.debug("request_many",
					"subject", subject,
					"replies", len(collected),
					"elapsed", time.Since(start),
					"outcome", "channel_closed",
				)
				return collected, nil
			}
			// NATS sends a status reply (HTTP 503 "No Responders Available
			// For Request") to the inbox when no subscribers exist for the
			// subject. The status-header convention is part of NATS's wire
			// protocol; the constant is not exported by nats.go, so the
			// header key is inlined. For RequestMany this is not an error —
			// the legitimate "all-quiet" outcome is the empty slice after
			// the window elapses (FR-110). Skip status replies; they carry
			// no application data.
			if msg.Header != nil && msg.Header.Get(natsStatusHdr) != "" {
				continue
			}
			collected = append(collected, msg.Data)
			if maxReplies > 0 && len(collected) >= maxReplies {
				log.debug("request_many",
					"subject", subject,
					"replies", len(collected),
					"elapsed", time.Since(start),
					"outcome", "cap",
				)
				return collected, nil
			}
		case <-deadline.C:
			log.debug("request_many",
				"subject", subject,
				"replies", len(collected),
				"elapsed", time.Since(start),
				"outcome", "window",
			)
			return collected, nil
		case <-ctx.Done():
			log.warn("request_many failed",
				"subject", subject,
				"category", "canceled",
				"cause", ctx.Err(),
			)
			return collected, ctx.Err()
		}
	}
}
