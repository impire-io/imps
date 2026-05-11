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

func boundarySpec(reasoning harness.ReasoningFn) harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "boundary",
		Version: "0.1.0",
		Channels: []harness.ChannelSpec{{
			Name:   "inbound",
			Source: harness.SubjectSource{Subject: "messages.in"},
			Decode: func(msg harness.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(any) (harness.Entity, error) { return "singleton", nil },
		}},
		Awareness: func(_ context.Context, decoded any, e harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Wake(decoded, e)
		},
		Reasoning: reasoning,
		Actions:   []string{"actions.out"},
	}
}

func TestOffWhitelistPublishRejected(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	publishErr := make(chan error, 1)
	spec := boundarySpec(func(ctx context.Context, _ any, _ harness.Entity, r harness.ReasoningContext) error {
		err := r.Publish(ctx, "not.allowed", []byte("nope"))
		publishErr <- err
		return err
	})

	imp, err := harness.NewImp(spec, nc, harness.WithSubjectPrefix("test"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	// Subscribe on a wildcard so we'd see ANY publish if it slipped through.
	gotAny := make(chan struct{}, 1)
	if _, err := nc.Subscribe("test.>", func(m *nats.Msg) {
		// Ignore the message we publish to messages.in below.
		if m.Subject == "test.messages.in" {
			return
		}
		gotAny <- struct{}{}
	}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("test.messages.in", []byte("trigger")); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-publishErr:
		var wl *harness.ErrWhitelistViolation
		if !errors.As(err, &wl) || wl.Subject != "not.allowed" {
			t.Fatalf("expected ErrWhitelistViolation{not.allowed}, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("reasoning never returned; metrics=%+v", imp.Metrics())
	}

	// No off-whitelist publish should have reached NATS.
	time.Sleep(100 * time.Millisecond)
	select {
	case <-gotAny:
		t.Fatal("rejected publish reached NATS — whitelist enforcement broken")
	default:
	}

	m := imp.Metrics()
	if m.WhitelistViolations == 0 {
		t.Fatalf("expected WhitelistViolations >= 1, got %+v", m)
	}
}

func TestWhitelistedPublishSucceeds(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	spec := boundarySpec(func(ctx context.Context, _ any, _ harness.Entity, r harness.ReasoningContext) error {
		return r.Publish(ctx, "actions.out", []byte("ok"))
	})

	imp, err := harness.NewImp(spec, nc, harness.WithSubjectPrefix("test"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("test.actions.out", func(m *nats.Msg) { got <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("test.messages.in", []byte("trigger")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-got:
		if string(data) != "ok" {
			t.Fatalf("got %q want %q", data, "ok")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout; metrics=%+v", imp.Metrics())
	}
	if v := imp.Metrics().WhitelistViolations; v != 0 {
		t.Fatalf("unexpected whitelist violations: %d", v)
	}
}
