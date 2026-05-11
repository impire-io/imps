// Echo is the worked example from specs/001-harness-core/quickstart.md.
// It subscribes to messages.in, awareness always wakes, reasoning publishes
// the payload back to actions.out — both on literal subjects, since the
// framework performs no subject transformation.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps/harness"
)

func main() {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer func() { _ = nc.Drain() }()

	spec := harness.ImpSpec{
		Name:    "echo",
		Version: "0.1.0",
		Channels: []harness.ChannelSpec{{
			Name:   "inbound",
			Source: harness.SubjectSource{Subject: "messages.in"},
			Decode: func(msg harness.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(_ any) (harness.Entity, error) {
				return harness.Entity("singleton"), nil
			},
		}},
		Awareness: func(_ context.Context, decoded any, entity harness.Entity, _ harness.AwarenessContext) harness.Verdict {
			return harness.Wake(decoded, entity)
		},
		Reasoning: func(ctx context.Context, reason any, _ harness.Entity, r harness.ReasoningContext) error {
			payload := []byte(reason.(string))
			return r.Publish(ctx, "actions.out", payload)
		},
	}

	imp, err := harness.NewImp(spec, nc,
		harness.WithLogger(slog.NewTextHandler(os.Stdout, nil)),
	)
	if err != nil {
		log.Fatalf("build imp: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := imp.Run(ctx); err != nil {
		log.Fatalf("run imp: %v", err)
	}
}
