package harness

import (
	"io"
	"log/slog"
	"time"
)

// runtimeOptions holds the harness's configurable runtime parameters. It is
// populated from the variadic Option list passed to NewImp; defaults are
// applied before any Option runs.
type runtimeOptions struct {
	drainWindow time.Duration
	logHandler  slog.Handler
}

func defaultRuntimeOptions() runtimeOptions {
	return runtimeOptions{
		drainWindow: 30 * time.Second,
		logHandler:  slog.NewTextHandler(io.Discard, nil),
	}
}

// Option configures the harness at construction. Options are applied
// left-to-right; later options override earlier ones.
type Option func(*runtimeOptions)

// WithDrainWindow sets the maximum time Shutdown will wait for in-flight
// reasoning to complete. Default is 30s.
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
