package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps/harness"
	"github.com/impire-io/imps/testutil/natstest"
)

// startBareImp starts an imp without injecting a default channel. Tests
// that drive Request/RequestMany from awareness or reasoning need full
// control over the imp's ChannelSpec.
func startBareImp(t *testing.T, spec harness.ImpSpec, opts ...harness.Option) (*harness.Imp, *nats.Conn, func()) {
	t.Helper()
	srv := natstest.New(t)
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatalf("nats.Connect: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	imp, err := harness.NewImp(spec, nc, opts...)
	if err != nil {
		t.Fatalf("NewImp: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !imp.Ready() {
		time.Sleep(10 * time.Millisecond)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("nc.Flush: %v", err)
	}

	cleanup := func() {
		cancel()
		select {
		case <-runErr:
		case <-time.After(3 * time.Second):
			t.Errorf("Run did not exit within 3s after cancel")
		}
	}
	return imp, nc, cleanup
}

// reasoningRequestSpec builds an imp whose reasoning calls r.Request on
// the given subject with the decoded payload and publishes the reply on
// "actions.out". The supplied reqOpts are forwarded to the Request call.
func reasoningRequestSpec(subject string, reqOpts ...harness.RequestOption) harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "request-reasoning",
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
			reply, err := r.Request(ctx, subject, reason.([]byte), reqOpts...)
			if err != nil {
				return r.Publish(ctx, "actions.err", []byte(err.Error()))
			}
			return r.Publish(ctx, "actions.out", reply)
		},
	}
}

// TestRequest_Reasoning_HappyPath — US-1 acceptance scenarios 1 and 2.
func TestRequest_Reasoning_HappyPath(t *testing.T) {
	imp, nc, cleanup := startBareImp(t, reasoningRequestSpec("knowledge.recall"))
	defer cleanup()

	if _, err := nc.Subscribe("knowledge.recall", func(m *nats.Msg) {
		_ = m.Respond(m.Data)
	}); err != nil {
		t.Fatal(err)
	}
	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("actions.out", func(m *nats.Msg) { got <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("messages.in", []byte("ping")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-got:
		if string(data) != "ping" {
			t.Fatalf("got reply %q, want %q", data, "ping")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for echoed reply; metrics=%+v", imp.Metrics())
	}

	// Allow the metrics increment to settle (RequestCalls is incremented
	// inside requestSingle before returning; the reasoning goroutine has
	// already finished by the time the actions.out callback fires).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if imp.Metrics().RequestCalls >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := imp.Metrics().RequestCalls; got != 1 {
		t.Fatalf("RequestCalls = %d, want 1", got)
	}
}

// TestRequest_Reasoning_PerCallTimeoutHonored — US-1 AS-2: the call respects
// WithRequestTimeout when supplied. (Counterpart timeout-failure assertions
// live in TestRequest_ErrRequestTimeout under US-5 / US-6.)
func TestRequest_Reasoning_PerCallTimeoutHonored(t *testing.T) {
	imp, nc, cleanup := startBareImp(t,
		reasoningRequestSpec("knowledge.recall", harness.WithRequestTimeout(50*time.Millisecond)),
	)
	defer cleanup()

	// Responder delays past the per-call timeout.
	if _, err := nc.Subscribe("knowledge.recall", func(m *nats.Msg) {
		time.Sleep(200 * time.Millisecond)
		_ = m.Respond(m.Data)
	}); err != nil {
		t.Fatal(err)
	}
	gotErr := make(chan []byte, 1)
	if _, err := nc.Subscribe("actions.err", func(m *nats.Msg) { gotErr <- m.Data }); err != nil {
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
	case msg := <-gotErr:
		elapsed := time.Since(start)
		// Tolerance: the dispatch path adds a handful of ms; 150ms is well
		// under the 200ms responder delay, so any return below that means
		// the per-call timeout fired first.
		if elapsed > 150*time.Millisecond {
			t.Fatalf("per-call timeout did not fire promptly: elapsed=%v error=%q", elapsed, msg)
		}
		if string(msg) == "" {
			t.Fatalf("expected non-empty timeout error string")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for timeout-error; metrics=%+v", imp.Metrics())
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
	// Sanity check: errors module reachable.
	_ = errors.Is
}

// awarenessRequestSpec builds an imp whose awareness calls a.Request on
// the given subject. The reply (or error) drives the verdict and is
// captured via the OnNote hook into notes.
func awarenessRequestSpec(
	subject string,
	notes chan<- any,
	reqOpts ...harness.RequestOption,
) harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "request-awareness",
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
		Awareness: func(ctx context.Context, decoded any, e harness.Entity, a harness.AwarenessContext) harness.Verdict {
			reply, err := a.Request(ctx, subject, decoded.([]byte), reqOpts...)
			if err != nil {
				var toErr *harness.ErrRequestTimeout
				var noResp *harness.ErrNoResponders
				switch {
				case errors.As(err, &toErr):
					return harness.Note("timeout:" + toErr.Subject)
				case errors.As(err, &noResp):
					return harness.Note("no_responders:" + noResp.Subject)
				default:
					return harness.Note("err:" + err.Error())
				}
			}
			if string(reply) == "ignore" {
				return harness.Ignore()
			}
			return harness.Wake(reply, e)
		},
		Reasoning: func(_ context.Context, _ any, _ harness.Entity, _ harness.ReasoningContext) error {
			return nil
		},
		OnNote: func(_ harness.Entity, payload any) {
			notes <- payload
		},
	}
}

// TestRequest_Awareness_HappyPath — US-3 AS-1: a.Request reply drives the
// verdict; the deterministic transformer's reply chooses Wake vs Ignore.
func TestRequest_Awareness_HappyPath(t *testing.T) {
	notes := make(chan any, 4)
	imp, nc, cleanup := startBareImp(t, awarenessRequestSpec(
		"embed.short", notes,
		harness.WithRequestTimeout(200*time.Millisecond),
	))
	defer cleanup()

	// Transformer: replies "ignore" if the input has length divisible by 2,
	// otherwise "wake".
	if _, err := nc.Subscribe("embed.short", func(m *nats.Msg) {
		if len(m.Data)%2 == 0 {
			_ = m.Respond([]byte("ignore"))
		} else {
			_ = m.Respond([]byte("wake"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	// Even length → Ignore → no Note delivered. Odd length → Wake.
	if err := nc.Publish("messages.in", []byte("abc")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-notes:
		// awareness returned Wake, but Wake does not deliver to OnNote;
		// the only way a note arrives is the error path. Fail loudly.
		t.Fatalf("unexpected note for wake verdict; metrics=%+v", imp.Metrics())
	case <-time.After(300 * time.Millisecond):
		// expected: no note for wake/ignore.
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if imp.Metrics().RequestCalls >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := imp.Metrics().RequestCalls; got < 1 {
		t.Fatalf("RequestCalls = %d, want >= 1", got)
	}
}

// TestRequest_Awareness_TimeoutInVerdict — US-3 AS-2: awareness still
// yields a verdict (Note carrying the degraded state) when its a.Request
// times out.
func TestRequest_Awareness_TimeoutInVerdict(t *testing.T) {
	notes := make(chan any, 4)
	imp, nc, cleanup := startBareImp(t, awarenessRequestSpec(
		"embed.short", notes,
		harness.WithRequestTimeout(50*time.Millisecond),
	))
	defer cleanup()

	if _, err := nc.Subscribe("embed.short", func(m *nats.Msg) {
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
	case note := <-notes:
		s, ok := note.(string)
		if !ok || s != "timeout:embed.short" {
			t.Fatalf("note = %v, want %q", note, "timeout:embed.short")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for timeout-note; metrics=%+v", imp.Metrics())
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if imp.Metrics().RequestTimeouts >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := imp.Metrics().RequestTimeouts; got < 1 {
		t.Fatalf("RequestTimeouts = %d, want >= 1", got)
	}
}

// TestSubjectsAreLiteral_Request — US-7 AS-1 for Request (both reasoning
// and awareness paths) plus the verbatim Subject field on
// *ErrNoResponders.
func TestSubjectsAreLiteral_Request(t *testing.T) {
	// Reasoning path: capture msg.Subject on a literal-subject responder
	// and assert it is byte-for-byte the declared subject.
	t.Run("reasoning_subject_byte_for_byte", func(t *testing.T) {
		_, nc, cleanup := startBareImp(t, reasoningRequestSpec("knowledge.recall"))
		defer cleanup()

		seen := make(chan string, 1)
		if _, err := nc.Subscribe("knowledge.recall", func(m *nats.Msg) {
			seen <- m.Subject
			_ = m.Respond(m.Data)
		}); err != nil {
			t.Fatal(err)
		}
		if err := nc.Flush(); err != nil {
			t.Fatal(err)
		}
		if err := nc.Publish("messages.in", []byte("hi")); err != nil {
			t.Fatal(err)
		}
		select {
		case subj := <-seen:
			if subj != "knowledge.recall" {
				t.Fatalf("captured subject = %q, want %q", subj, "knowledge.recall")
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout")
		}
	})

	// Negative variant: ErrNoResponders carries the verbatim subject.
	t.Run("err_no_responders_subject_verbatim", func(t *testing.T) {
		replies := make(chan []byte, 1)
		errs := make(chan error, 1)
		_, nc, cleanup := startBareImp(t, reasoningCallSpec(
			"knowledge.recall", replies, errs, false,
			harness.WithRequestTimeout(500*time.Millisecond),
		))
		defer cleanup()
		if _, err := nc.Subscribe("unrelated", func(_ *nats.Msg) {}); err != nil {
			t.Fatal(err)
		}
		if err := nc.Flush(); err != nil {
			t.Fatal(err)
		}
		if err := nc.Publish("messages.in", []byte("hi")); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-errs:
			var noResp *harness.ErrNoResponders
			if !errors.As(err, &noResp) {
				t.Fatalf("err = %T %v, want *ErrNoResponders", err, err)
			}
			if noResp.Subject != "knowledge.recall" {
				t.Fatalf("noResp.Subject = %q, want %q", noResp.Subject, "knowledge.recall")
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout")
		}
	})

	// Awareness path: a.Request must also send on the verbatim subject.
	t.Run("awareness_subject_byte_for_byte", func(t *testing.T) {
		notes := make(chan any, 1)
		_, nc, cleanup := startBareImp(t, awarenessRequestSpec(
			"embed.short", notes,
			harness.WithRequestTimeout(500*time.Millisecond),
		))
		defer cleanup()

		seen := make(chan string, 1)
		if _, err := nc.Subscribe("embed.short", func(m *nats.Msg) {
			seen <- m.Subject
			_ = m.Respond([]byte("ignore"))
		}); err != nil {
			t.Fatal(err)
		}
		if err := nc.Flush(); err != nil {
			t.Fatal(err)
		}
		if err := nc.Publish("messages.in", []byte("hi")); err != nil {
			t.Fatal(err)
		}
		select {
		case subj := <-seen:
			if subj != "embed.short" {
				t.Fatalf("captured subject = %q, want %q", subj, "embed.short")
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout")
		}
	})
}
