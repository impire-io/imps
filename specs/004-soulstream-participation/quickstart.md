# Quickstart: An Imp on the Soulstream

A complete imp that observes a topic, notes other personas' turns without
thinking, and posts a turn from thinking. Assumes a NATS server with
JetStream and a realm already provisioned by the soulstream operator tooling
(`soulstream provision`); this module never provisions.

```go
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"

	imps "github.com/impire-io/imps"
	soulimps "github.com/impire-io/imps/soulstream"
)

const topicPath = "vat-q2-2026-x7m2" // an existing topic's path

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	// The imp's soulstream identity: its own connection, a realm, a persona.
	// The participant never closes nc — the imp owns it.
	participant, err := soulimps.NewParticipant(ctx, nc, "myrealm", "vat-watcher")
	if err != nil {
		log.Fatal(err)
	}

	spec := imps.ImpSpec{
		Name:    "vat-watcher",
		Version: "0.1.0",

		// Declaring the channel IS joining: baseline first, full history,
		// then live — through the same dispatch path as any channel.
		Channels: []imps.ChannelSpec{
			soulimps.TopicChannel(topicPath),
		},

		// Awareness observes ops (three decoded headers) and stays cheap.
		Awareness: func(_ context.Context, decoded any, _ imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			op := decoded.(soulimps.Op)
			switch {
			case op.Type == "turn.post" && op.Author != "vat-watcher":
				// Lightweight contribution, no thinking: becomes a
				// comment.add anchored to this op via the bridge below.
				return imps.Note(soulimps.Noted{AnchorOp: op.ID, Body: "seen — on it"})
			case op.Type == "work.open":
				return imps.Think(op, imps.Entity(topicPath))
			default:
				return imps.Ignore()
			}
		},

		// The bridge turns Noted payloads into anchored comments; anything
		// else falls through to your own note handling (nil here = dropped).
		OnNote: soulimps.NoteBridge(participant, nil, func(e imps.Entity, n soulimps.Noted, err error) {
			log.Printf("note failed on %s: %v", e, err)
		}),

		// Thinking is a full participant: turns, comments, topic lifecycle.
		Thinking: func(ctx context.Context, reason any, entity imps.Entity, t imps.ThinkingContext) error {
			h := participant.Topic(string(entity))
			_, err := h.PostTurn(ctx, "claiming this work item — vat-watcher")
			return err
		},
	}

	imp, err := imps.NewImp(spec, nc)
	if err != nil {
		log.Fatal(err)
	}
	// Run blocks; ctx cancellation shuts down and deletes the ephemeral
	// consumer — which is all "leaving" means.
	if err := imp.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

## Variations

- **Durable resume**: `soulimps.TopicChannel(topicPath, soulimps.WithDurable("vat-watcher-vat"))`
  — a restarted imp receives exactly what it missed instead of replaying from
  the baseline.
- **Signed contributions**: `soulimps.NewParticipant(ctx, nc, "myrealm", "vat-watcher", soulimps.WithSigner(key))`
  — every turn and note verifies as this persona.
- **Custom decoding**: `soulimps.WithDecoder(...)` when awareness needs
  payload fields; keep it cheap — materialisation belongs in thinking
  (`participant.Topic(path).Materialise(ctx)`).

## Driving each error category deterministically

| Category | How to reproduce | Expected |
|---|---|---|
| Unprovisioned realm | Point the imp at a JetStream server without the `SOULSTREAM` stream | `Run` fails startup with the harness's `ErrStreamNotFound`; no partial subscriptions remain |
| Write without persona | `NewParticipant(ctx, nc, "myrealm", "")`, then any post | The owner library's "a persona is required to post" error |
| Cross-persona authorship | Attempt to author as another persona through the owner library | Refused by the attribution guard |
| Malformed note | `imps.Note(soulimps.Noted{})` | Bridge error callback fires; nothing published |
| Non-bridge note payload | `imps.Note("just local")` | Delegated to `next` (or dropped); nothing published |
| Archived topic | Post to an archived topic | Owner library's `ErrTopicArchived`, surfaced unchanged |
