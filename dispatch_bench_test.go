package imps

import (
	"context"
	"sync/atomic"
	"testing"
)

// BenchmarkDispatchOverhead measures the per-message dispatch overhead
// across various inflight-reasoning and tracked-entity counts. The harness
// commitment (SC-010) is sub-linear growth in both dimensions; the bench
// exists to catch obvious regressions, not to publish absolute numbers.
//
// The bench bypasses NATS — it drives dispatch directly through the
// internal pipeline so we measure the harness's own overhead, not the
// substrate's.
func BenchmarkDispatchOverhead(b *testing.B) {
	cases := []struct {
		name              string
		inflightReasoning int
		trackedEntities   int
	}{
		{"baseline", 0, 10},
		{"inflight=100", 100, 10},
		{"inflight=1000", 1000, 10},
		{"entities=1000", 0, 1000},
		{"inflight=1000_entities=1000", 1000, 1000},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			imp, ch, release := dispatchBenchSetup(b, c.inflightReasoning, c.trackedEntities)
			defer release()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				imp.dispatch(ctx, ch, Message{
					Subject: "bench",
					Data:    []byte("payload"),
				})
			}
		})
	}
}

// BenchmarkDispatchThinkReturns measures dispatch return latency when the
// verdict is Think. Reasoning blocks until released; the benchmark exits
// far before that — proving dispatch returns regardless of reasoning
// latency (FR-016, FR-020).
func BenchmarkDispatchThinkReturns(b *testing.B) {
	imp, ch, release := dispatchBenchSetup(b, 0, 1)
	defer release()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		imp.dispatch(context.Background(), ch, Message{
			Subject: "bench",
			Data:    []byte("payload"),
		})
	}
}

// dispatchBenchSetup builds an Imp wired up for in-process dispatch with a
// fake NATS connection (we never touch the substrate in dispatch). Returns
// the imp, a channel state, and a release func that drains held reasoning.
func dispatchBenchSetup(b *testing.B, preheatInflight, entityCap int) (*Imp, *channelState, func()) {
	b.Helper()
	release := make(chan struct{})

	var entityIdx atomic.Int64
	spec := ImpSpec{
		Name:    "bench",
		Version: "1",
		States: []StateShape{{
			Name:    "counter",
			Factory: func() any { return new(int) },
			Cap:     entityCap + 100,
		}},
		Channels: []ChannelSpec{{
			Name:   "in",
			Source: SubjectSource{Subject: "bench"},
			Decode: func(Message) (any, error) { return entityIdx.Add(1), nil },
			ExtractEntity: func(decoded any) (Entity, error) {
				return Entity(intToBytes(int(decoded.(int64) % int64(entityCap)))), nil
			},
		}},
		Awareness: func(_ context.Context, _ any, e Entity, _ AwarenessContext) Verdict {
			return Think("bench", e)
		},
		Reasoning: func(ctx context.Context, _ any, _ Entity, _ ReasoningContext) error {
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		},
	}

	// Manual construction — bypass NewImp's nil-conn check so the bench
	// can run without a real NATS connection.
	imp := &Imp{spec: spec, opts: defaultRuntimeOptions()}
	if err := imp.bootRuntime(); err != nil {
		b.Fatalf("bootRuntime: %v", err)
	}
	ch := &channelState{spec: spec.Channels[0], subject: "bench"}

	// Pre-heat in-flight reasoning by directly launching the configured
	// number of goroutines that block on `release`.
	for i := 0; i < preheatInflight; i++ {
		imp.launchReasoning("preheat", "preheat-entity")
	}

	return imp, ch, func() {
		close(release)
		imp.runtime().reasoningWG.Wait()
	}
}

func intToBytes(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
