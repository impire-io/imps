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

// modeSpec returns the SAME imp source used in both modes — the goal of
// US7 is that the developer-facing source is mode-independent (FR-032,
// SC-008).
func modeSpec() harness.ImpSpec {
	return harness.ImpSpec{
		Name:    "modes",
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
		Reasoning: func(ctx context.Context, reason any, _ harness.Entity, r harness.ReasoningContext) error {
			return r.Publish(ctx, "actions.out", []byte(reason.(string)))
		},
		Actions: []string{"actions.out"},
	}
}

func TestNonPlatformModeResolution(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	imp, err := harness.NewImp(modeSpec(), nc, harness.WithSubjectPrefix("tenantA.imps.demo"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("tenantA.imps.demo.actions.out", func(m *nats.Msg) { got <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("tenantA.imps.demo.messages.in", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-got:
		if string(data) != "hello" {
			t.Fatalf("got %q want hello", data)
		}
	case <-time.After(time.Second):
		t.Fatalf("non-platform action not received; metrics=%+v", imp.Metrics())
	}
	if pfx := imp.Identity().SubjectPrefix; pfx != "tenantA.imps.demo" {
		t.Fatalf("identity prefix mismatch: %q", pfx)
	}
}

func TestPlatformModeResolution(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	imp, err := harness.NewImp(modeSpec(), nc,
		harness.WithSubjectPrefix("platform"),
		harness.WithPlatformMode("ABCD1234EFGH"),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()
	waitReady(t, imp)

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("platform.ABCD1234EFGH.actions.out", func(m *nats.Msg) { got <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("platform.ABCD1234EFGH.messages.in", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-got:
		if string(data) != "hello" {
			t.Fatalf("got %q want hello", data)
		}
	case <-time.After(time.Second):
		t.Fatalf("platform action not received; metrics=%+v", imp.Metrics())
	}
	if pfx := imp.Identity().SubjectPrefix; pfx != "platform.ABCD1234EFGH" {
		t.Fatalf("identity prefix mismatch: %q", pfx)
	}
}

func TestPlatformModeMissingImporterPK(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	imp, err := harness.NewImp(modeSpec(), nc,
		harness.WithSubjectPrefix("platform"),
		harness.WithPlatformMode(""),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := imp.Run(ctx)

	var ci *harness.ErrConfigInvalid
	if !errors.As(runErr, &ci) || ci.Field != "importer_account_pk" {
		t.Fatalf("expected ErrConfigInvalid{importer_account_pk}, got %v", runErr)
	}
}

func TestNonPlatformModeMissingPrefix(t *testing.T) {
	srv := natstest.New(t)
	nc, _ := nats.Connect(srv.URL())
	t.Cleanup(func() { nc.Close() })

	imp, err := harness.NewImp(modeSpec(), nc) // no prefix option
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := imp.Run(ctx)

	var ci *harness.ErrConfigInvalid
	if !errors.As(runErr, &ci) || ci.Field != "prefix" {
		t.Fatalf("expected ErrConfigInvalid{prefix}, got %v", runErr)
	}
}
