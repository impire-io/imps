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
	registry  *registry
	resolver  *resolver
	whitelist *whitelist
	metrics   *metrics
	logger    logger

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
// declaration, the resolved subject (post-resolution), and the substrate
// handle so shutdown can unsubscribe cleanly.
type channelState struct {
	spec            ChannelSpec
	resolvedSubject string
	subscription    *nats.Subscription

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
		"subject_prefix", rt.identity.SubjectPrefix,
	)

	select {
	case <-ctx.Done():
		return i.Shutdown(context.Background())
	case <-rt.stopped:
		return rt.shutdownErr
	}
}

// bootRuntime creates the runtime bag exactly once. Validates option
// invariants that depend on the prefix / platform-mode combination
// (FR-033).
func (i *Imp) bootRuntime() error {
	var err error
	i.rtOnce.Do(func() {
		res, rerr := newResolver(i.opts.subjectPrefix, i.opts.platformMode, i.opts.importerAccountPK)
		if rerr != nil {
			err = rerr
			return
		}
		m := newMetrics()
		reg := newRegistry(i.spec.States)
		wl := newWhitelist(i.spec.Actions)
		lg := newLogger(i.opts.logHandler)

		rCtx, rCancel := context.WithCancel(context.Background())

		rt := &runtime{
			registry:        reg,
			resolver:        res,
			whitelist:       wl,
			metrics:         m,
			logger:          lg,
			reasoningCtx:    rCtx,
			reasoningCancel: rCancel,
			state:           lifecycle.New(),
			stopped:         make(chan struct{}),
		}
		rt.awareness = &awarenessCtx{registry: reg}
		rt.reasoning = &reasoningCtx{
			registry:  reg,
			resolver:  res,
			whitelist: wl,
			conn:      i.nc,
			metrics:   m,
			logger:    lg,
		}
		rt.identity = ImpIdentity{
			Name:          i.spec.Name,
			Version:       i.spec.Version,
			SubjectPrefix: res.resolvedPrefix(),
		}
		i.rtPtr.Store(rt)
	})
	return err
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

// Identity returns the running imp's name, version, and resolved subject
// prefix. Available during Running, Draining, and Stopped states. In the
// Failed state, returns the partial values that were resolved before the
// failure (typically Name and Version from the spec; SubjectPrefix may be
// empty).
func (i *Imp) Identity() ImpIdentity {
	if rt := i.runtime(); rt != nil {
		return rt.identity
	}
	return ImpIdentity{Name: i.spec.Name, Version: i.spec.Version}
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
