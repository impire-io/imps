package schedule

import (
	imps "github.com/impire-io/imps"
)

// The substrate's scheduling header names, defined once. Write-side headers
// are built by Register; read-side headers are decoded into Tick. The
// server (nats-server ≥ the pinned version) is the authority on their
// semantics; this package only types them.
const (
	headerSchedule         = "Nats-Schedule"
	headerScheduleTarget   = "Nats-Schedule-Target"
	headerScheduleTTL      = "Nats-Schedule-TTL"
	headerScheduleTimeZone = "Nats-Schedule-Time-Zone"
	headerScheduleRollup   = "Nats-Schedule-Rollup"
	headerScheduleSource   = "Nats-Schedule-Source"

	headerScheduler    = "Nats-Scheduler"
	headerScheduleNext = "Nats-Schedule-Next"
)

// Tick is the imp's header-level view of one schedule firing.
type Tick struct {
	// Subject is the target subject the tick arrived on (the default
	// entity).
	Subject string
	// Scheduler is the schedule subject that produced this tick — the
	// Nats-Scheduler provenance header, present on every server-emitted
	// tick.
	Scheduler string
	// Next is the Nats-Schedule-Next header, verbatim: the RFC3339 time of
	// the next firing on a repeating schedule, or the server's purge marker
	// on a final firing. Informational.
	Next string
}

// decodeTick is Channel's default Decoder: headers only, never an error —
// a tick with missing headers is delivered zero-valued for awareness to
// judge.
func decodeTick(m imps.Message) (any, error) {
	return Tick{
		Subject:   m.Subject,
		Scheduler: m.Headers.Get(headerScheduler),
		Next:      m.Headers.Get(headerScheduleNext),
	}, nil
}

// extractTarget is Channel's default EntityExtractor: the target subject is
// the entity for every tick on the channel.
func extractTarget(target string) imps.EntityExtractor {
	return func(any) (imps.Entity, error) {
		return imps.Entity(target), nil
	}
}

// defaultChannelName derives the name shown in harness logs and metrics.
func defaultChannelName(target string) string {
	return "schedule:" + target
}
