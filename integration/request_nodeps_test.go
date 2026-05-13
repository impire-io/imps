package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps"
)

// TestNoRequest_FootprintUnchanged — SC-107. An imp that issues no
// Request / RequestMany / Publish must produce the same metrics movement
// as in 001-harness-core: the four new counters stay at zero; existing
// 001 counters move per the pre-existing observability contract.
func TestNoRequest_FootprintUnchanged(t *testing.T) {
	notes := make(chan struct{}, 8)
	spec := imps.ImpSpec{
		Name:    "no-request",
		Version: "0.1.0",
		Channels: []imps.ChannelSpec{{
			Name:   "inbound",
			Source: imps.SubjectSource{Subject: "messages.in"},
			Decode: func(msg imps.Message) (any, error) {
				return msg.Data, nil
			},
			ExtractEntity: func(any) (imps.Entity, error) {
				return imps.Entity("singleton"), nil
			},
		}},
		// Awareness always returns Note (delivers to OnNote; no thinking).
		Awareness: func(_ context.Context, decoded any, _ imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			return imps.Note(decoded)
		},
		Thinking: func(_ context.Context, _ any, _ imps.Entity, _ imps.ThinkingContext) error {
			return nil
		},
		OnNote: func(_ imps.Entity, _ any) {
			notes <- struct{}{}
		},
	}

	imp, nc, cleanup := startBareImp(t, spec)
	defer cleanup()

	const N = 5
	for i := 0; i < N; i++ {
		if err := nc.Publish("messages.in", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	delivered := 0
	for delivered < N && time.Now().Before(deadline) {
		select {
		case <-notes:
			delivered++
		case <-time.After(50 * time.Millisecond):
		}
	}
	if delivered != N {
		t.Fatalf("delivered = %d, want %d", delivered, N)
	}

	m := imp.Metrics()
	if m.RequestCalls != 0 {
		t.Errorf("RequestCalls = %d, want 0", m.RequestCalls)
	}
	if m.RequestManyCalls != 0 {
		t.Errorf("RequestManyCalls = %d, want 0", m.RequestManyCalls)
	}
	if m.RequestNoResponders != 0 {
		t.Errorf("RequestNoResponders = %d, want 0", m.RequestNoResponders)
	}
	if m.RequestTimeouts != 0 {
		t.Errorf("RequestTimeouts = %d, want 0", m.RequestTimeouts)
	}

	// Existing 001 counters still move as before: each message produced
	// exactly one Note, no Think, no decode/extract failure.
	if got := m.NotesDelivered; got != N {
		t.Errorf("NotesDelivered = %d, want %d", got, N)
	}
	if m.ThinksDispatched != 0 {
		t.Errorf("ThinksDispatched = %d, want 0", m.ThinksDispatched)
	}
	if m.DecodeFailures != 0 {
		t.Errorf("DecodeFailures = %d, want 0", m.DecodeFailures)
	}
	if m.ExtractionFailures != 0 {
		t.Errorf("ExtractionFailures = %d, want 0", m.ExtractionFailures)
	}

	// Sanity: nats.Msg is still importable; the test file doesn't depend
	// on it but keeping the import slot avoids drift if future maintainers
	// add NATS-direct assertions here.
	_ = nats.Msg{}
}
