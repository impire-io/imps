# Quickstart: An Imp on the Clock

A complete imp that attends periodic work without owning a timer. Assumes an
operator-provisioned stream with scheduling enabled, e.g.:

```sh
nats stream add SCHED --subjects 'schedules.>,ticks.>' \
  --allow-msg-schedules --allow-msg-ttl --defaults
```

```go
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	imps "github.com/impire-io/imps"
	"github.com/impire-io/imps/schedule"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatal(err)
	}

	// Register (or replace) the schedule: typed headers, one publish.
	// The TTL is the explicit stale-tick governor: ticks older than 10m
	// at wake were expired by the server and never arrive. Omitting it
	// means the full backlog accumulates — a deliberate choice.
	if err := schedule.Register(ctx, js,
		"schedules.reconcile", "@every 5m", "ticks.reconcile",
		schedule.WithTickTTL(10*time.Minute),
	); err != nil {
		log.Fatal(err)
	}

	spec := imps.ImpSpec{
		Name:    "reconciler",
		Version: "0.1.0",
		Channels: []imps.ChannelSpec{
			// The tick channel IS an ordinary stream channel; the durable
			// cursor makes catch-up after a cold gap exact (and TTL-pruned).
			schedule.Channel("SCHED", "ticks.reconcile",
				schedule.WithDurable("reconciler-ticks")),
		},
		Awareness: func(_ context.Context, decoded any, entity imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			t := decoded.(schedule.Tick)
			// Every tick names its producer — route on provenance if one
			// channel carries several schedules' targets.
			return imps.Think(t, entity)
		},
		Thinking: func(ctx context.Context, reason any, _ imps.Entity, tc imps.ThinkingContext) error {
			// the periodic work itself
			return nil
		},
	}

	imp, err := imps.NewImp(spec, nc)
	if err != nil {
		log.Fatal(err)
	}
	if err := imp.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

## Driving each behavior deterministically

| Behavior | How to see it | Expected |
|---|---|---|
| Live ticks | Run the imp, wait a cadence | Tick dispatched like any message, `Tick.Scheduler == "schedules.reconcile"` |
| Cold catch-up | Stop the imp through several firings, restart | Durable consumer replays the retained backlog through the same channel |
| Stale-tick governance | Same, with `WithTickTTL` shorter than the gap | Only the unexpired tail arrives — the server expired the rest |
| Full accumulation | Same, without `WithTickTTL` | The whole backlog arrives |
| Replacement | `Register` again with a new pattern | Next firing follows the new pattern; no duplicate schedule |
| Removal | `schedule.Deregister(ctx, js, "SCHED", "schedules.reconcile")` | No further ticks; emitted ticks untouched |
| Missing stream | Point the imp at an unprovisioned server | `Run` fails with the harness's `ErrStreamNotFound` |
| Fail-fast validation | `Register` with empty pattern or target | Error before any substrate contact |
