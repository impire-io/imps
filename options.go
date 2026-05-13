package imps

import (
	"io"
	"log/slog"
	"time"
)

// runtimeOptions holds the harness's configurable runtime parameters. It is
// populated from the variadic Option list passed to NewImp; defaults are
// applied before any Option runs.
type runtimeOptions struct {
	drainWindow              time.Duration
	logHandler               slog.Handler
	defaultRequestTimeout    time.Duration
	defaultRequestManyWindow time.Duration
}

func defaultRuntimeOptions() runtimeOptions {
	return runtimeOptions{
		drainWindow:              30 * time.Second,
		logHandler:               slog.NewTextHandler(io.Discard, nil),
		defaultRequestTimeout:    5 * time.Second,
		defaultRequestManyWindow: 1 * time.Second,
	}
}

// Option configures the harness at construction. Options are applied
// left-to-right; later options override earlier ones.
type Option func(*runtimeOptions)

// WithDrainWindow sets the maximum time Shutdown will wait for in-flight
// thinking to complete. Default is 30s.
func WithDrainWindow(d time.Duration) Option {
	return func(o *runtimeOptions) {
		o.drainWindow = d
	}
}

// WithLogger sets the slog.Handler the harness uses for structured
// logging. Default is a discard handler.
func WithLogger(h slog.Handler) Option {
	return func(o *runtimeOptions) {
		if h != nil {
			o.logHandler = h
		}
	}
}

// WithDefaultRequestTimeout sets the default per-call timeout applied to
// Request invocations that do not supply WithRequestTimeout. Default is
// 5 seconds. A non-positive duration is rejected at Run with
// *ErrConfigInvalid.
func WithDefaultRequestTimeout(d time.Duration) Option {
	return func(o *runtimeOptions) {
		o.defaultRequestTimeout = d
	}
}

// WithDefaultRequestManyWindow sets the default collection window applied
// to RequestMany invocations that do not supply WithRequestManyWindow.
// Default is 1 second. A non-positive duration is rejected at Run with
// *ErrConfigInvalid.
func WithDefaultRequestManyWindow(d time.Duration) Option {
	return func(o *runtimeOptions) {
		o.defaultRequestManyWindow = d
	}
}
