package integration_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps"
)

// twoCallReasoningSpec drives two Request calls per reasoning invocation:
// first against fastSubject (uses harness default), then against slowSubject
// (uses per-call WithRequestTimeout(perCallTO)). Captures both outcomes.
func twoCallReasoningSpec(
	fastSubject, slowSubject string,
	perCallTO time.Duration,
	fastReplies chan<- []byte,
	slowErrs chan<- error,
) imps.ImpSpec {
	return imps.ImpSpec{
		Name:    "request-timeout",
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
		Awareness: func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			return imps.Think(decoded, e)
		},
		Reasoning: func(ctx context.Context, reason any, _ imps.Entity, r imps.ReasoningContext) error {
			reply, err := r.Request(ctx, fastSubject, reason.([]byte))
			if err != nil {
				slowErrs <- err
				return nil
			}
			fastReplies <- reply

			_, err = r.Request(ctx, slowSubject, reason.([]byte),
				imps.WithRequestTimeout(perCallTO),
			)
			slowErrs <- err
			return nil
		},
	}
}

// TestRequest_PerCallTimeoutPrecedence — US-6 AS-1 and AS-2.
func TestRequest_PerCallTimeoutPrecedence(t *testing.T) {
	fastReplies := make(chan []byte, 1)
	slowErrs := make(chan error, 2)

	_, nc, cleanup := startBareImp(t, twoCallReasoningSpec(
		"fast", "slow", 5*time.Millisecond,
		fastReplies, slowErrs,
	),
		imps.WithDefaultRequestTimeout(time.Second),
	)
	defer cleanup()

	if _, err := nc.Subscribe("fast", func(m *nats.Msg) {
		time.Sleep(10 * time.Millisecond)
		_ = m.Respond([]byte("fast-ok"))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := nc.Subscribe("slow", func(m *nats.Msg) {
		time.Sleep(200 * time.Millisecond)
		_ = m.Respond([]byte("slow-late"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("messages.in", []byte("ping")); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-fastReplies:
		if string(got) != "fast-ok" {
			t.Fatalf("fast reply = %q, want %q", got, "fast-ok")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for fast reply")
	}

	select {
	case err := <-slowErrs:
		var toErr *imps.ErrRequestTimeout
		if !errors.As(err, &toErr) {
			t.Fatalf("slow err = %T %v, want *ErrRequestTimeout", err, err)
		}
		if toErr.Timeout != 5*time.Millisecond {
			t.Fatalf("toErr.Timeout = %v, want 5ms (per-call must override default)", toErr.Timeout)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for slow err")
	}
}

// TestRequest_NoRetryOnTimeout — US-6 AS-3 / SC-106: a timeout on the
// caller's side must NOT cause the harness to publish a second request
// on the same subject.
func TestRequest_NoRetryOnTimeout(t *testing.T) {
	replies := make(chan []byte, 1)
	errs := make(chan error, 1)

	_, nc, cleanup := startBareImp(t, reasoningCallSpec(
		"count.me", replies, errs, false,
		imps.WithRequestTimeout(50*time.Millisecond),
	))
	defer cleanup()

	var received atomic.Int32
	if _, err := nc.Subscribe("count.me", func(m *nats.Msg) {
		received.Add(1)
		time.Sleep(200 * time.Millisecond)
		_ = m.Respond([]byte("late"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("messages.in", []byte("ping")); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errs:
		var toErr *imps.ErrRequestTimeout
		if !errors.As(err, &toErr) {
			t.Fatalf("err = %T %v, want *ErrRequestTimeout", err, err)
		}
	case <-replies:
		t.Fatalf("unexpected success; expected timeout")
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}

	// Wait long enough for any hypothetical retry to land on the counting
	// responder before sampling. The responder's reply is 200ms after
	// receiving; another 300ms guard is generous.
	time.Sleep(500 * time.Millisecond)

	if got := received.Load(); got != 1 {
		t.Fatalf("responder received %d requests, want exactly 1 (no retry)", got)
	}
}

// TestRequestMany_PerCallWindowAndMax — US-6 AS-4 and AS-5. The window
// override and the max-cap both override harness defaults for one call.
func TestRequestMany_PerCallWindowAndMax(t *testing.T) {
	replies := make(chan [][]byte, 1)
	errs := make(chan error, 1)

	_, nc, cleanup := startBareImp(t, reasoningManySpec(
		"override.fanout", replies, errs, "",
		imps.WithRequestManyWindow(300*time.Millisecond),
		imps.WithRequestManyMax(2),
	),
		imps.WithDefaultRequestManyWindow(50*time.Millisecond),
	)
	defer cleanup()

	for i := 0; i < 4; i++ {
		if _, err := nc.Subscribe("override.fanout", func(m *nats.Msg) {
			_ = m.Respond([]byte("ok"))
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := nc.Publish("messages.in", []byte("ping")); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-replies:
		elapsed := time.Since(start)
		if len(got) != 2 {
			t.Fatalf("len(replies) = %d, want 2 (cap=2)", len(got))
		}
		// Cap fires well before the per-call 300ms window; if the harness
		// default (50ms) had been used and the cap not honored, behavior
		// would be different. The cap returning quickly is the assertion.
		if elapsed >= 300*time.Millisecond {
			t.Fatalf("cap did not short-circuit: elapsed=%v", elapsed)
		}
	case err := <-errs:
		t.Fatalf("err = %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}
