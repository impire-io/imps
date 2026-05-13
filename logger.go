package imps

import "log/slog"

// logger wraps a *slog.Logger with the helper methods the harness uses.
// Wrapping keeps the call sites short and allows the harness to evolve
// the level/event taxonomy without touching every call.
type logger struct {
	l *slog.Logger
}

func newLogger(h slog.Handler) logger {
	return logger{l: slog.New(h)}
}

func (lg logger) debug(msg string, kv ...any) { lg.l.Debug(msg, kv...) }
func (lg logger) info(msg string, kv ...any)  { lg.l.Info(msg, kv...) }
func (lg logger) warn(msg string, kv ...any)  { lg.l.Warn(msg, kv...) }
func (lg logger) error(msg string, kv ...any) { lg.l.Error(msg, kv...) }
