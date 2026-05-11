package integration_test

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps/harness"
	"github.com/impire-io/imps/testutil/natstest"
)

// TestSustainedAwarenessUnderLoad drives sustained channel publishes for
// thousands of distinct entities while reasoning remains in flight. The
// assertion (SC-011) is that awareness keeps dispatching — measured here
// as the per-message metric counters keeping pace with the publish count.
func TestSustainedAwarenessUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test")
	}
	const (
		entities = 2000
		bursts   = 2
	)

	srv := natstest.New(t)
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })

	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	var awarenessSeen atomic.Int64

	spec := harness.ImpSpec{
		Name:    "stress",
		Version: "1",
		Channels: []harness.ChannelSpec{{
			Name:   "in",
			Source: harness.SubjectSource{Subject: "messages.in"},
			Decode: func(msg harness.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(decoded any) (harness.Entity, error) {
				return harness.Entity(decoded.(string)), nil
			},
		}},
		Awareness: func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			awarenessSeen.Add(1)
			return harness.Wake(decoded, e)
		},
		Reasoning: func(ctx context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error {
			// Hold every reasoning invocation so it stays in flight while
			// we drive more awareness through the dispatcher.
			select {
			case <-hold:
			case <-ctx.Done():
			}
			return nil
		},
		Actions: []string{"actions.out"},
	}

	imp, err := harness.NewImp(spec, nc,
		harness.WithSubjectPrefix("test"),
		harness.WithDrainWindow(2*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	for round := 0; round < bursts; round++ {
		for i := 0; i < entities; i++ {
			if err := nc.Publish("test.messages.in", []byte(strconv.Itoa(i))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	expected := int64(entities * bursts)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if awarenessSeen.Load() >= expected {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if seen := awarenessSeen.Load(); seen < expected {
		t.Fatalf("awareness backpressured under load: saw %d / %d (metrics=%+v)", seen, expected, imp.Metrics())
	}
	if v := imp.Metrics().InflightReasoning; v < int64(entities) {
		t.Fatalf("expected reasoning to stay in flight (>= %d), got %d", entities, v)
	}
}
