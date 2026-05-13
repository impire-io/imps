package integration_test

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps"
	"github.com/impire-io/imps/testutil/natstest"
)

// TestSustainedAwarenessUnderLoad drives sustained channel publishes for
// thousands of distinct entities while thinking remains in flight. The
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

	spec := imps.ImpSpec{
		Name:    "stress",
		Version: "1",
		Channels: []imps.ChannelSpec{{
			Name:   "in",
			Source: imps.SubjectSource{Subject: "messages.in"},
			Decode: func(msg imps.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(decoded any) (imps.Entity, error) {
				return imps.Entity(decoded.(string)), nil
			},
		}},
		Awareness: func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			awarenessSeen.Add(1)
			return imps.Think(decoded, e)
		},
		Thinking: func(ctx context.Context, _ any, _ imps.Entity, _ imps.ThinkingContext) error {
			// Hold every thinking invocation so it stays in flight while
			// we drive more awareness through the dispatcher.
			select {
			case <-hold:
			case <-ctx.Done():
			}
			return nil
		},
	}

	imp, err := imps.NewImp(spec, nc,

		imps.WithDrainWindow(2*time.Second),
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
			if err := nc.Publish("messages.in", []byte(strconv.Itoa(i))); err != nil {
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
	if v := imp.Metrics().InflightThinking; v < int64(entities) {
		t.Fatalf("expected thinking to stay in flight (>= %d), got %d", entities, v)
	}
}
