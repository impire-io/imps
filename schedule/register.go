package schedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// registerConfig accumulates RegisterOption effects.
type registerConfig struct {
	ttl      time.Duration
	ttlSet   bool
	timeZone string
	body     []byte
	source   string
	rollup   bool
}

// RegisterOption customises a registered schedule.
type RegisterOption func(*registerConfig)

// WithTickTTL makes every emitted tick expire on the server after ttl (the
// stream needs AllowMsgTTL): an imp waking from a gap longer than ttl
// replays only the unexpired tail. ttl must be > 0. OMITTING this option
// means the full backlog accumulates and is delivered on wake — choose
// deliberately.
func WithTickTTL(ttl time.Duration) RegisterOption {
	return func(c *registerConfig) { c.ttl, c.ttlSet = ttl, true }
}

// WithTimeZone sets the schedule's time zone (cron patterns only, per the
// server; not supported for "@every").
func WithTimeZone(tz string) RegisterOption {
	return func(c *registerConfig) { c.timeZone = tz }
}

// WithBody sets the tick payload (default empty).
func WithBody(body []byte) RegisterOption {
	return func(c *registerConfig) { c.body = body }
}

// WithSource makes each tick carry the last message on the given subject
// instead of Body — a server-side "emit the latest state" schedule.
func WithSource(subject string) RegisterOption {
	return func(c *registerConfig) { c.source = subject }
}

// WithRollup marks each tick as a per-subject rollup on its target.
func WithRollup() RegisterOption {
	return func(c *registerConfig) { c.rollup = true }
}

// Register publishes a schedule: one headered message on scheduleSubject,
// firing pattern into target. One schedule per subject is the server's
// semantics — registering an existing subject REPLACES its schedule.
// pattern is validated by the server; its grammar (read from the pinned
// server): "@every <dur>" (minimum 1s), "@at <RFC3339>" one-shots,
// SIX-field cron with a seconds field ("0 0 12 * * *"), and the predefined
// "@hourly"/"@daily"/"@weekly"/"@monthly"/"@yearly" forms. This package
// fails fast only on what needs no substrate: empty pattern or target, or
// a non-positive TTL.
//
// Registration is a write: call it from thinking or operator tooling,
// never from awareness.
func Register(ctx context.Context, js jetstream.JetStream, scheduleSubject, pattern, target string, opts ...RegisterOption) error {
	if scheduleSubject == "" {
		return errors.New("schedule: schedule subject required")
	}
	if pattern == "" {
		return errors.New("schedule: pattern required")
	}
	if target == "" {
		return errors.New("schedule: target subject required")
	}
	var cfg registerConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.ttlSet && cfg.ttl <= 0 {
		return errors.New("schedule: tick TTL must be > 0")
	}

	msg := &nats.Msg{Subject: scheduleSubject, Data: cfg.body, Header: nats.Header{}}
	msg.Header.Set(headerSchedule, pattern)
	msg.Header.Set(headerScheduleTarget, target)
	if cfg.ttl > 0 {
		msg.Header.Set(headerScheduleTTL, cfg.ttl.String())
	}
	if cfg.timeZone != "" {
		msg.Header.Set(headerScheduleTimeZone, cfg.timeZone)
	}
	if cfg.source != "" {
		msg.Header.Set(headerScheduleSource, cfg.source)
	}
	if cfg.rollup {
		msg.Header.Set(headerScheduleRollup, "sub")
	}
	if _, err := js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("schedule: register %s: %w", scheduleSubject, err)
	}
	return nil
}

// Deregister removes the schedule by purging its subject in stream: future
// firings stop; ticks already emitted are unaffected.
func Deregister(ctx context.Context, js jetstream.JetStream, stream, scheduleSubject string) error {
	if stream == "" || scheduleSubject == "" {
		return errors.New("schedule: stream and schedule subject required")
	}
	st, err := js.Stream(ctx, stream)
	if err != nil {
		return fmt.Errorf("schedule: deregister %s: %w", scheduleSubject, err)
	}
	if err := st.Purge(ctx, jetstream.WithPurgeSubject(scheduleSubject)); err != nil {
		return fmt.Errorf("schedule: deregister %s: %w", scheduleSubject, err)
	}
	return nil
}
