package harness

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/imps/internal/lifecycle"
)

// Imp is the runtime handle for a configured imp. It is created by NewImp
// and started by Run. Identity, Metrics, and Shutdown are safe to call from
// any goroutine.
type Imp struct {
	spec ImpSpec
	nc   *nats.Conn
	opts runtimeOptions

	// rt holds runtime-only state. nil before Run installs it; written
	// exactly once via rtOnce; readers use rtPtr.Load() so observers on
	// other goroutines see a fully-initialized runtime under the race
	// detector. The convenience accessor i.runtime() hides the load.
	rtOnce sync.Once
	rtPtr  atomic.Pointer[runtime]
}

// runtime returns the populated runtime, or nil if Run has not yet been
// invoked. Safe to call from any goroutine.
func (i *Imp) runtime() *runtime { return i.rtPtr.Load() }

// runtime is the unexported bag of runtime state populated by Run. All
// fields are concurrency-safe in their own way (atomics, sync.WaitGroup,
// or read-only after construction).
type runtime struct {
	registry *registry
	metrics  *metrics
	logger   logger

	awareness *awarenessCtx
	reasoning *reasoningCtx

	// reasoningCtx is the context handed to user reasoning functions.
	// Cancelled when shutdown begins so reasoning can exit cooperatively.
	reasoningCtx    context.Context
	reasoningCancel context.CancelFunc

	reasoningWG sync.WaitGroup

	channels []*channelState

	// js is the lazily-initialized JetStream context, populated the first
	// time a StreamSource channel is bound. Nil for subject-only imps.
	js      jetstream.JetStream
	streams []jetstream.Stream

	state    *lifecycle.Machine
	identity ImpIdentity

	shutdownOnce sync.Once
	shutdownErr  error
	stopped      chan struct{}
}

// channelState is the per-channel runtime record. It tracks the source
// declaration, the substrate handle so shutdown can unsubscribe
// cleanly, and (for stream channels) the consumer name and stream.
type channelState struct {
	spec         ChannelSpec
	subject      string // the literal subject (or filter subject) the channel binds
	subscription *nats.Subscription

	// Stream-channel fields. Populated only when spec.Source is a
	// StreamSource.
	stream        string
	consumerName  string
	ephemeral     bool
	streamConsume jetstream.ConsumeContext
}

// NewImp validates the supplied ImpSpec, applies the variadic options,
// and returns a non-running Imp handle. The caller supplies the NATS
// connection — the harness does not connect itself.
func NewImp(spec ImpSpec, nc *nats.Conn, opts ...Option) (*Imp, error) {
	if nc == nil {
		return nil, &ErrConfigInvalid{Field: "nats.Conn", Reason: "nil"}
	}
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	o := defaultRuntimeOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return &Imp{spec: spec, nc: nc, opts: o}, nil
}

// Run establishes every declared subscription, registers identity, then
// dispatches messages until ctx is cancelled or Shutdown is called. Run
// blocks until shutdown completes.
func (i *Imp) Run(ctx context.Context) error {
	if err := i.bootRuntime(); err != nil {
		return err
	}
	rt := i.runtime()

	if !rt.state.CompareAndSwap(lifecycle.StateCreated, lifecycle.StateStarting) {
		return fmt.Errorf("harness: Run already invoked (state %s)", rt.state.Get())
	}

	if err := i.start(); err != nil {
		rt.state.Set(lifecycle.StateFailed)
		rt.logger.error("startup failed", "err", err)
		// Roll back any partial subscriptions established before failure.
		i.unwind()
		return err
	}

	rt.state.Set(lifecycle.StateRunning)
	rt.logger.info("imp ready",
		"name", i.spec.Name,
		"version", i.spec.Version,
	)

	select {
	case <-ctx.Done():
		return i.Shutdown(context.Background())
	case <-rt.stopped:
		return rt.shutdownErr
	}
}

// bootRuntime creates the runtime bag exactly once.
func (i *Imp) bootRuntime() error {
	var bootErr error
	i.rtOnce.Do(func() {
		if i.opts.defaultRequestTimeout <= 0 {
			bootErr = &ErrConfigInvalid{
				Field:  "default_request_timeout",
				Reason: "non-positive",
			}
			return
		}
		if i.opts.defaultRequestManyWindow <= 0 {
			bootErr = &ErrConfigInvalid{
				Field:  "default_request_many_window",
				Reason: "non-positive",
			}
			return
		}

		m := newMetrics()
		reg := newRegistry(i.spec.States)
		lg := newLogger(i.opts.logHandler)

		rCtx, rCancel := context.WithCancel(context.Background())

		rt := &runtime{
			registry:        reg,
			metrics:         m,
			logger:          lg,
			reasoningCtx:    rCtx,
			reasoningCancel: rCancel,
			state:           lifecycle.New(),
			stopped:         make(chan struct{}),
		}
		rt.awareness = &awarenessCtx{
			registry:              reg,
			conn:                  i.nc,
			metrics:               m,
			logger:                lg,
			defaultRequestTimeout: i.opts.defaultRequestTimeout,
		}
		rt.reasoning = &reasoningCtx{
			registry:                 reg,
			conn:                     i.nc,
			metrics:                  m,
			logger:                   lg,
			defaultRequestTimeout:    i.opts.defaultRequestTimeout,
			defaultRequestManyWindow: i.opts.defaultRequestManyWindow,
		}
		rt.identity = ImpIdentity{
			Name:    i.spec.Name,
			Version: i.spec.Version,
		}
		i.rtPtr.Store(rt)
	})
	return bootErr
}

// Shutdown initiates graceful shutdown: stops accepting new messages,
// cancels in-flight reasoning's context, waits up to drain_window for
// reasoning to complete, and returns no later than drain_window + ε.
//
// Calling Shutdown more than once is safe. The supplied ctx is honored
// only as an upper bound — the drain window also applies.
func (i *Imp) Shutdown(_ context.Context) error {
	rt := i.runtime()
	if rt == nil {
		// Not started; nothing to do.
		return nil
	}
	rt.shutdownOnce.Do(func() {
		rt.state.Set(lifecycle.StateDraining)
		rt.logger.info("imp shutdown begin", "drain_window", i.opts.drainWindow.String())

		// Phase 1: stop accepting new messages.
		i.unwind()

		// Phase 2: cancel reasoning context so cooperative reasoning can exit.
		rt.reasoningCancel()

		// Phase 3: wait up to drain_window for in-flight reasoning to complete.
		rt.shutdownErr = i.waitDrain()

		rt.state.Set(lifecycle.StateStopped)
		rt.logger.info("imp shutdown end",
			"pending_reasoning", rt.metrics.InflightReasoning.Load(),
		)
		close(rt.stopped)
	})
	<-rt.stopped
	return rt.shutdownErr
}

// waitDrain blocks for up to opts.drainWindow on the reasoning WaitGroup.
// Returns nil if all reasoning completed; returns context.DeadlineExceeded
// (joined with no other error) if the deadline expired.
func (i *Imp) waitDrain() error {
	rt := i.runtime()
	done := make(chan struct{})
	go func() {
		rt.reasoningWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(i.opts.drainWindow):
		rt.logger.warn("drain deadline exceeded",
			"pending_reasoning", rt.metrics.InflightReasoning.Load(),
		)
		return errors.Join(context.DeadlineExceeded, errors.New("harness: drain window exceeded"))
	}
}

// Identity returns the running imp's name and version. Available in
// every lifecycle state.
func (i *Imp) Identity() ImpIdentity {
	if rt := i.runtime(); rt != nil {
		return rt.identity
	}
	return ImpIdentity{Name: i.spec.Name, Version: i.spec.Version}
}

// Ready reports whether the imp has finished startup and is dispatching
// messages. Useful as a readiness signal for tests and for callers that
// need to wait for subscriptions to register before publishing.
func (i *Imp) Ready() bool {
	rt := i.runtime()
	if rt == nil {
		return false
	}
	return rt.state.Get() == lifecycle.StateRunning
}

// Metrics returns a non-resetting snapshot of harness counters. Safe to
// call concurrently with dispatch.
func (i *Imp) Metrics() Metrics {
	rt := i.runtime()
	if rt == nil {
		return Metrics{}
	}
	return rt.metrics.snapshot()
}
