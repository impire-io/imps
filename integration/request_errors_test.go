package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps/harness"
)

// reasoningCallSpec routes a single Request call through reasoning and
// captures its (reply, error) outcome on caller-supplied channels.
func reasoningCallSpec(
	subject string,
	replies chan<- []byte,
	errs chan<- error,
	cancelMid bool,
	reqOpts ...harness.RequestOption,
) harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "request-errors",
		Version: "0.1.0",
		Channels: []harness.ChannelSpec{{
			Name:   "inbound",
			Source: harness.SubjectSource{Subject: "messages.in"},
			Decode: func(msg harness.Message) (any, error) {
				return msg.Data, nil
			},
			ExtractEntity: func(any) (harness.Entity, error) {
				return harness.Entity("singleton"), nil
			},
		}},
		Awareness: func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Wake(decoded, e)
		},
		Reasoning: func(ctx context.Context, reason any, _ harness.Entity, r harness.ReasoningContext) error {
			callCtx := ctx
			if cancelMid {
				c, cancel := context.WithCancel(ctx)
				callCtx = c
				go func() {
					time.Sleep(20 * time.Millisecond)
					cancel()
				}()
			}
			reply, err := r.Request(callCtx, subject, reason.([]byte), reqOpts...)
			if err != nil {
				errs <- err
				return nil
			}
			replies <- reply
			return nil
		},
	}
}

// TestRequest_ErrNoResponders — US-5 AS-1.
func TestRequest_ErrNoResponders(t *testing.T) {
	replies := make(chan []byte, 1)
	errs := make(chan error, 1)

	imp, nc, cleanup := startBareImp(t, reasoningCallSpec(
		"nobody.home", replies, errs, false,
		harness.WithRequestTimeout(2*time.Second),
	))
	defer cleanup()

	// No subscriber registered on nobody.home; NATS short-circuits with
	// ErrNoResponders. Subscribe an unrelated subject so the connection
	// itself isn't idle.
	if _, err := nc.Subscribe("unrelated", func(_ *nats.Msg) {}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := nc.Publish("messages.in", []byte("ping")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-replies:
		t.Fatalf("unexpected reply; expected ErrNoResponders")
	case err := <-errs:
		elapsed := time.Since(start)
		var noResp *harness.ErrNoResponders
		if !errors.As(err, &noResp) {
			t.Fatalf("err = %T %v, want *ErrNoResponders", err, err)
		}
		if noResp.Subject != "nobody.home" {
			t.Fatalf("noResp.Subject = %q, want %q", noResp.Subject, "nobody.home")
		}
		if elapsed >= time.Second {
			t.Fatalf("ErrNoResponders did not short-circuit: elapsed=%v", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout; metrics=%+v", imp.Metrics())
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if imp.Metrics().RequestNoResponders >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := imp.Metrics().RequestNoResponders; got != 1 {
		t.Fatalf("RequestNoResponders = %d, want 1", got)
	}
}

// TestRequest_ErrRequestTimeout — US-5 AS-2.
func TestRequest_ErrRequestTimeout(t *testing.T) {
	replies := make(chan []byte, 1)
	errs := make(chan error, 1)

	imp, nc, cleanup := startBareImp(t, reasoningCallSpec(
		"slow", replies, errs, false,
		harness.WithRequestTimeout(50*time.Millisecond),
	))
	defer cleanup()

	if _, err := nc.Subscribe("slow", func(m *nats.Msg) {
		time.Sleep(200 * time.Millisecond)
		_ = m.Respond(m.Data)
	}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := nc.Publish("messages.in", []byte("ping")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-replies:
		t.Fatalf("unexpected reply; expected ErrRequestTimeout")
	case err := <-errs:
		elapsed := time.Since(start)
		var toErr *harness.ErrRequestTimeout
		if !errors.As(err, &toErr) {
			t.Fatalf("err = %T %v, want *ErrRequestTimeout", err, err)
		}
		if toErr.Subject != "slow" {
			t.Fatalf("toErr.Subject = %q, want %q", toErr.Subject, "slow")
		}
		if toErr.Timeout != 50*time.Millisecond {
			t.Fatalf("toErr.Timeout = %v, want 50ms", toErr.Timeout)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("errors.Is(err, context.DeadlineExceeded) = false; want true")
		}
		if elapsed < 50*time.Millisecond || elapsed > 200*time.Millisecond {
			t.Fatalf("elapsed=%v not in [50ms, 200ms]", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout; metrics=%+v", imp.Metrics())
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if imp.Metrics().RequestTimeouts >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := imp.Metrics().RequestTimeouts; got != 1 {
		t.Fatalf("RequestTimeouts = %d, want 1", got)
	}
}

// TestRequest_CtxCanceled — US-5 AS-3.
func TestRequest_CtxCanceled(t *testing.T) {
	replies := make(chan []byte, 1)
	errs := make(chan error, 1)

	_, nc, cleanup := startBareImp(t, reasoningCallSpec(
		"slowcancel", replies, errs, true,
		harness.WithRequestTimeout(500*time.Millisecond),
	))
	defer cleanup()

	if _, err := nc.Subscribe("slowcancel", func(m *nats.Msg) {
		time.Sleep(200 * time.Millisecond)
		_ = m.Respond(m.Data)
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
	case <-replies:
		t.Fatalf("unexpected reply; expected context.Canceled")
	case err := <-errs:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("errors.Is(err, context.Canceled) = false; want true (err=%v)", err)
		}
		var toErr *harness.ErrRequestTimeout
		if errors.As(err, &toErr) {
			t.Fatalf("cancellation should not surface as *ErrRequestTimeout (got %v)", toErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for cancellation error")
	}
}
