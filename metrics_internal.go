package imps

import "sync/atomic"

// metrics holds the harness's atomic counter set. Each counter is a
// uint64 atomic; InflightThinking is an int64 gauge incremented before a
// thinking goroutine starts and decremented when it returns or panics.
//
// Snapshot returns a non-resetting Metrics value. It is safe to call
// concurrently with dispatch; individual loads are atomic but the snapshot
// is not transactionally consistent across counters.
type metrics struct {
	InflightThinking   atomic.Int64
	DecodeFailures     atomic.Uint64
	ExtractionFailures atomic.Uint64
	AwarenessPanics    atomic.Uint64
	ThinkingPanics     atomic.Uint64
	ThinkingErrors     atomic.Uint64
	NotesDelivered     atomic.Uint64
	ThinksDispatched   atomic.Uint64
	IgnoredVerdicts    atomic.Uint64
	NakTotal           atomic.Uint64

	RequestCalls        atomic.Uint64
	RequestManyCalls    atomic.Uint64
	RequestNoResponders atomic.Uint64
	RequestTimeouts     atomic.Uint64
}

func newMetrics() *metrics { return &metrics{} }

func (m *metrics) snapshot() Metrics {
	return Metrics{
		InflightThinking:    m.InflightThinking.Load(),
		DecodeFailures:      m.DecodeFailures.Load(),
		ExtractionFailures:  m.ExtractionFailures.Load(),
		AwarenessPanics:     m.AwarenessPanics.Load(),
		ThinkingPanics:      m.ThinkingPanics.Load(),
		ThinkingErrors:      m.ThinkingErrors.Load(),
		NotesDelivered:      m.NotesDelivered.Load(),
		ThinksDispatched:    m.ThinksDispatched.Load(),
		IgnoredVerdicts:     m.IgnoredVerdicts.Load(),
		NakTotal:            m.NakTotal.Load(),
		RequestCalls:        m.RequestCalls.Load(),
		RequestManyCalls:    m.RequestManyCalls.Load(),
		RequestNoResponders: m.RequestNoResponders.Load(),
		RequestTimeouts:     m.RequestTimeouts.Load(),
	}
}
