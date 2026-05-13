// Echo is the worked example from specs/001-harness-core/quickstart.md.
// It subscribes to messages.in, awareness always escalates to thinking,
// and thinking publishes the payload back to actions.out — both on literal
// subjects, since the framework performs no subject transformation.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps"
)

func main() {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer func() { _ = nc.Drain() }()

	spec := imps.ImpSpec{
		Name:    "echo",
		Version: "0.1.0",
		Channels: []imps.ChannelSpec{{
			Name:   "inbound",
			Source: imps.SubjectSource{Subject: "messages.in"},
			Decode: func(msg imps.Message) (any, error) {
				return string(msg.Data), nil
			},
			ExtractEntity: func(_ any) (imps.Entity, error) {
				return imps.Entity("singleton"), nil
			},
		}},
		Awareness: func(_ context.Context, decoded any, entity imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			return imps.Think(decoded, entity)
		},
		Thinking: func(ctx context.Context, reason any, _ imps.Entity, r imps.ThinkingContext) error {
			payload := []byte(reason.(string))
			return r.Publish(ctx, "actions.out", payload)
		},
	}

	imp, err := imps.NewImp(spec, nc,
		imps.WithLogger(slog.NewTextHandler(os.Stdout, nil)),
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
