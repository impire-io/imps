package imps

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// runEmbeddedServer is a tiny clone of testutil/natstest used in this
// internal-package test file. We avoid importing testutil/natstest here
// because that package imports harness, which would create a cycle.
func runEmbeddedServer(t *testing.T) string {
	t.Helper()
	srv, err := server.NewServer(&server.Options{
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 true,
		NoSigs:                true,
		DisableShortFirstPing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("server not ready")
	}
	t.Cleanup(srv.Shutdown)
	return srv.ClientURL()
}

// TestSlogEventsEmitted asserts that the lifecycle and decode-failure
// events listed in contracts/observability.md "Logger" actually arrive
// at the configured slog.Handler.
func TestSlogEventsEmitted(t *testing.T) {
	url := runEmbeddedServer(t)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	spec := ImpSpec{
		Name:    "obs",
		Version: "1",
		Channels: []ChannelSpec{{
			Name:   "in",
			Source: SubjectSource{Subject: "messages.in"},
			Decode: func(msg Message) (any, error) {
				if string(msg.Data) == "bad" {
					return nil, errors.New("decode boom")
				}
				return string(msg.Data), nil
			},
			ExtractEntity: func(any) (Entity, error) { return "x", nil },
		}},
		Awareness: func(_ context.Context, decoded any, e Entity, _ AwarenessContext) Verdict {
			return Think(decoded, e)
		},
		Reasoning: func(ctx context.Context, _ any, _ Entity, r ReasoningContext) error {
			return r.Publish(ctx, "actions.out", []byte("x"))
		},
	}

	imp, err := NewImp(spec, nc,
		WithLogger(handler),
		WithDrainWindow(500*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !imp.Ready() {
		time.Sleep(10 * time.Millisecond)
	}

	if err := nc.Publish("messages.in", []byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("messages.in", []byte("good")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	// Allow events to flush.
	time.Sleep(200 * time.Millisecond)

	cancel()
	<-runErr

	output := buf.String()
	for _, want := range []string{
		"imp ready",
		"channel ready",
		"decode failure",
		"imp shutdown begin",
		"imp shutdown end",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected slog output to contain %q; got:\n%s", want, output)
		}
	}
}
