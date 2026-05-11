package harness

import "sync/atomic"

// metrics holds the harness's atomic counter set. Each counter is a
// uint64 atomic; InflightReasoning is an int64 gauge incremented before a
// reasoning goroutine starts and decremented when it returns or panics.
//
// Snapshot returns a non-resetting Metrics value. It is safe to call
// concurrently with dispatch; individual loads are atomic but the snapshot
// is not transactionally consistent across counters.
type metrics struct {
	InflightReasoning  atomic.Int64
	DecodeFailures     atomic.Uint64
	ExtractionFailures atomic.Uint64
	AwarenessPanics    atomic.Uint64
	ReasoningPanics    atomic.Uint64
	ReasoningErrors    atomic.Uint64
	NotesDelivered     atomic.Uint64
	WakesDispatched    atomic.Uint64
	IgnoredVerdicts    atomic.Uint64
	NakTotal           atomic.Uint64
}

func newMetrics() *metrics { return &metrics{} }

func (m *metrics) snapshot() Metrics {
	return Metrics{
		InflightReasoning:  m.InflightReasoning.Load(),
		DecodeFailures:     m.DecodeFailures.Load(),
		ExtractionFailures: m.ExtractionFailures.Load(),
		AwarenessPanics:    m.AwarenessPanics.Load(),
		ReasoningPanics:    m.ReasoningPanics.Load(),
		ReasoningErrors:    m.ReasoningErrors.Load(),
		NotesDelivered:     m.NotesDelivered.Load(),
		WakesDispatched:    m.WakesDispatched.Load(),
		IgnoredVerdicts:    m.IgnoredVerdicts.Load(),
		NakTotal:           m.NakTotal.Load(),
	}
}
