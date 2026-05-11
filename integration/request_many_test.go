package integration_test

import (
	"context"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps/harness"
)

// reasoningManySpec builds an imp whose reasoning calls
// r.RequestMany(subject, payload, manyOpts...) and forwards both the
// collected replies and any error to caller-supplied channels for the
// test to assert on. Optionally it also performs a Publish to assert
// US-4 (c) — reasoning has the full outbound surface.
func reasoningManySpec(
	subject string,
	replies chan<- [][]byte,
	errs chan<- error,
	alsoPublishSubject string,
	manyOpts ...harness.RequestManyOption,
) harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "request-many",
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
			got, err := r.RequestMany(ctx, subject, reason.([]byte), manyOpts...)
			if err != nil {
				errs <- err
				return err
			}
			replies <- got
			if alsoPublishSubject != "" {
				return r.Publish(ctx, alsoPublishSubject, []byte("ok"))
			}
			return nil
		},
	}
}

// TestReasoning_HasFullSurface — US-4 AS-3: reasoning successfully invokes
// r.RequestMany against one responder and r.Publish against an unrelated
// subject. Confirms the methods exist and work on ReasoningContext.
func TestReasoning_HasFullSurface(t *testing.T) {
	replies := make(chan [][]byte, 1)
	errs := make(chan error, 1)

	imp, nc, cleanup := startBareImp(t, reasoningManySpec(
		"health.ping", replies, errs, "actions.out",
		harness.WithRequestManyWindow(200*time.Millisecond),
	))
	defer cleanup()

	if _, err := nc.Subscribe("health.ping", func(m *nats.Msg) {
		_ = m.Respond([]byte("up"))
	}); err != nil {
		t.Fatal(err)
	}
	gotPublish := make(chan []byte, 1)
	if _, err := nc.Subscribe("actions.out", func(m *nats.Msg) { gotPublish <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("messages.in", []byte("ping")); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-replies:
		if len(got) != 1 || string(got[0]) != "up" {
			t.Fatalf("replies = %q, want [\"up\"]", got)
		}
	case err := <-errs:
		t.Fatalf("RequestMany returned error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for RequestMany replies; metrics=%+v", imp.Metrics())
	}

	select {
	case data := <-gotPublish:
		if string(data) != "ok" {
			t.Fatalf("publish data = %q, want \"ok\"", data)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for Publish to actions.out")
	}
}

// TestRequestMany_HappyPath — US-2 AS-1.
func TestRequestMany_HappyPath(t *testing.T) {
	replies := make(chan [][]byte, 1)
	errs := make(chan error, 1)

	imp, nc, cleanup := startBareImp(t, reasoningManySpec(
		"health.ping", replies, errs, "",
		harness.WithRequestManyWindow(200*time.Millisecond),
	))
	defer cleanup()

	for i := 0; i < 3; i++ {
		id := strconv.Itoa(i)
		if _, err := nc.Subscribe("health.ping", func(m *nats.Msg) {
			_ = m.Respond([]byte("r" + id))
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
		if len(got) != 3 {
			t.Fatalf("replies = %d, want 3 (%q)", len(got), got)
		}
		var ss []string
		for _, b := range got {
			ss = append(ss, string(b))
		}
		sort.Strings(ss)
		want := []string{"r0", "r1", "r2"}
		for i, w := range want {
			if ss[i] != w {
				t.Fatalf("sorted replies %v, want %v", ss, want)
			}
		}
		// Window honored: must be at least 200ms (waited for window).
		if elapsed < 200*time.Millisecond {
			t.Fatalf("window not honored: elapsed=%v < 200ms", elapsed)
		}
		// And within a generous upper bound.
		if elapsed > 600*time.Millisecond {
			t.Fatalf("window exceeded by too much: elapsed=%v", elapsed)
		}
	case err := <-errs:
		t.Fatalf("RequestMany error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout; metrics=%+v", imp.Metrics())
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if imp.Metrics().RequestManyCalls >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := imp.Metrics().RequestManyCalls; got != 1 {
		t.Fatalf("RequestManyCalls = %d, want 1", got)
	}
}

// TestRequestMany_MaxCapEarlyExit — US-2 AS-2 / FR-111.
func TestRequestMany_MaxCapEarlyExit(t *testing.T) {
	replies := make(chan [][]byte, 1)
	errs := make(chan error, 1)

	_, nc, cleanup := startBareImp(t, reasoningManySpec(
		"health.fanout", replies, errs, "",
		harness.WithRequestManyWindow(500*time.Millisecond),
		harness.WithRequestManyMax(3),
	))
	defer cleanup()

	for i := 0; i < 5; i++ {
		id := strconv.Itoa(i)
		if _, err := nc.Subscribe("health.fanout", func(m *nats.Msg) {
			_ = m.Respond([]byte("r" + id))
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
		if len(got) != 3 {
			t.Fatalf("len(replies) = %d, want 3", len(got))
		}
		// Cap must fire well before the window.
		if elapsed >= 400*time.Millisecond {
			t.Fatalf("cap did not short-circuit: elapsed=%v (window=500ms)", elapsed)
		}
	case err := <-errs:
		t.Fatalf("RequestMany error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}

// TestRequestMany_WindowElapseNoResponders — US-2 AS-3 / FR-110.
func TestRequestMany_WindowElapseNoResponders(t *testing.T) {
	replies := make(chan [][]byte, 1)
	errs := make(chan error, 1)

	_, nc, cleanup := startBareImp(t, reasoningManySpec(
		"health.empty", replies, errs, "",
		harness.WithRequestManyWindow(100*time.Millisecond),
	))
	defer cleanup()

	// Subscribe an unrelated subject so NATS isn't "no responders for
	// anyone reachable"; we want responders-for-other-subject but none
	// for our fan-out subject. The substrate would otherwise short-
	// circuit the publish with ErrNoResponders before the window opens.
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
	case got := <-replies:
		elapsed := time.Since(start)
		if len(got) != 0 {
			t.Fatalf("len(replies) = %d, want 0 (got %q)", len(got), got)
		}
		// Window honored.
		if elapsed < 100*time.Millisecond {
			t.Fatalf("window not honored: elapsed=%v", elapsed)
		}
		if elapsed > 500*time.Millisecond {
			t.Fatalf("window exceeded by too much: elapsed=%v", elapsed)
		}
	case err := <-errs:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}

// TestRequestMany_InboxCleanup — FR-113. Drive several RequestMany calls
// through the imp and assert the connection's subscription count returns
// to its baseline. We use direct reasoning-context calls instead of a
// full dispatch flow so we can probe nc.NumSubscriptions deterministically.
func TestRequestMany_InboxCleanup(t *testing.T) {
	// Reuse the dispatch-driven path: each scenario above runs a single
	// RequestMany call, then the imp is shut down. The remaining
	// subscriptions on the parent nc.Subscribe (responder + messages.in)
	// are unrelated to the temporary inbox. Here we simulate three calls
	// in sequence and assert the leak count.
	replies := make(chan [][]byte, 4)
	errs := make(chan error, 4)

	var driven atomic.Int32
	spec := harness.ImpSpec{
		Name:    "inbox-cleanup",
		Version: "0.1.0",
		Channels: []harness.ChannelSpec{{
			Name:   "inbound",
			Source: harness.SubjectSource{Subject: "cleanup.in"},
			Decode: func(msg harness.Message) (any, error) { return msg.Data, nil },
			ExtractEntity: func(any) (harness.Entity, error) {
				return harness.Entity(strconv.Itoa(int(driven.Add(1)))), nil
			},
		}},
		Awareness: func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Wake(decoded, e)
		},
		Reasoning: func(ctx context.Context, _ any, _ harness.Entity, r harness.ReasoningContext) error {
			got, err := r.RequestMany(ctx, "cleanup.out", nil,
				harness.WithRequestManyWindow(60*time.Millisecond),
			)
			if err != nil {
				errs <- err
				return err
			}
			replies <- got
			return nil
		},
	}

	_, nc, cleanup := startBareImp(t, spec)
	defer cleanup()

	// One responder; the others are "no reply, window elapses" by design.
	if _, err := nc.Subscribe("cleanup.out", func(m *nats.Msg) {
		_ = m.Respond([]byte("ok"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	baseSubs := nc.NumSubscriptions()

	for i := 0; i < 3; i++ {
		if err := nc.Publish("cleanup.in", []byte("go")); err != nil {
			t.Fatal(err)
		}
		select {
		case <-replies:
		case err := <-errs:
			t.Fatalf("call %d error: %v", i, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("call %d timeout", i)
		}
	}

	// Inbox subscriptions are short-lived; allow a brief moment for the
	// substrate to reflect Unsubscribe before sampling.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if nc.NumSubscriptions() == baseSubs {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("inbox leak: NumSubscriptions=%d, baseline=%d", nc.NumSubscriptions(), baseSubs)
}

// TestSubjectsAreLiteral_RequestMany_Publish — US-7 AS-1 for RequestMany
// and Publish. Both methods must send on the declared subject verbatim.
func TestSubjectsAreLiteral_RequestMany_Publish(t *testing.T) {
	replies := make(chan [][]byte, 1)
	errs := make(chan error, 1)

	_, nc, cleanup := startBareImp(t, reasoningManySpec(
		"health.ping", replies, errs, "actions.out",
		harness.WithRequestManyWindow(200*time.Millisecond),
	))
	defer cleanup()

	seenMany := make(chan string, 1)
	if _, err := nc.Subscribe("health.ping", func(m *nats.Msg) {
		seenMany <- m.Subject
		_ = m.Respond([]byte("up"))
	}); err != nil {
		t.Fatal(err)
	}
	seenPub := make(chan string, 1)
	if _, err := nc.Subscribe("actions.out", func(m *nats.Msg) {
		seenPub <- m.Subject
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
	case subj := <-seenMany:
		if subj != "health.ping" {
			t.Fatalf("RequestMany captured subject = %q, want %q", subj, "health.ping")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for RequestMany capture")
	}

	select {
	case subj := <-seenPub:
		if subj != "actions.out" {
			t.Fatalf("Publish captured subject = %q, want %q", subj, "actions.out")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for Publish capture")
	}
}
