// Echo example end-to-end smoke test that mirrors the integration-test
// pattern from quickstart.md.
package main

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps"
	"github.com/impire-io/imps/testutil/natstest"
)

func TestEchoEndToEnd(t *testing.T) {
	srv := natstest.New(t)
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })

	spec := imps.ImpSpec{
		Name:    "echo",
		Version: "0.1.0",
		Channels: []imps.ChannelSpec{{
			Name:   "inbound",
			Source: imps.SubjectSource{Subject: "messages.in"},
			Decode: func(msg imps.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(any) (imps.Entity, error) { return "singleton", nil },
		}},
		Awareness: func(_ context.Context, decoded any, e imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			return imps.Think(decoded, e)
		},
		Reasoning: func(ctx context.Context, reason any, _ imps.Entity, r imps.ReasoningContext) error {
			return r.Publish(ctx, "actions.out", []byte(reason.(string)))
		},
	}

	imp, err := imps.NewImp(spec, nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = imp.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !imp.Ready() {
		time.Sleep(10 * time.Millisecond)
	}

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("actions.out", func(m *nats.Msg) { got <- m.Data }); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := nc.Publish("messages.in", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-got:
		if string(data) != "hello" {
			t.Fatalf("got %q want hello", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for action")
	}
}
