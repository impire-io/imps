# Quickstart: An Imp That Sleeps

A complete imp whose per-entity state survives restarts, wakes with true
elapsed time, and stays memory-bounded. Assumes a NATS server with JetStream
and an operator-provisioned KV bucket (e.g. `nats kv add imp-state`).

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
	"github.com/impire-io/imps/persist"
)

// CustomerView is time-dependent durable state: loss on restart would be a
// bug (durable tier), and Idle ages with wall clock (wake hook).
type CustomerView struct {
	Opens int           `json:"opens"`
	Idle  time.Duration `json:"idle"`
}

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
	kv, err := js.KeyValue(ctx, "imp-state") // operator-provisioned
	if err != nil {
		log.Fatal(err)
	}
	backend := persist.KVBackend(kv)

	// The durable tier: bounded residency, write-through, wake on rehydration.
	customers := persist.NewStore[CustomerView]("customers", backend,
		persist.WithBound[CustomerView](128),
		persist.WithWake(func(_ imps.Entity, elapsed time.Duration, st CustomerView) CustomerView {
			st.Idle += elapsed // advance-to-now: pure in elapsed
			return st
		}),
	)

	// The imp-level sleep clock: how long was the whole imp down?
	beacon := persist.NewBeacon("customer-watcher", backend)
	if slept, ok, err := beacon.SleptFor(ctx); err != nil {
		log.Fatal(err)
	} else if ok {
		log.Printf("slept %v — imp-level wake runs before dispatch starts", slept)
	}

	spec := imps.ImpSpec{
		Name:    "customer-watcher",
		Version: "0.1.0",
		Channels: []imps.ChannelSpec{{
			Name:   "opens",
			Source: imps.SubjectSource{Subject: "crm.opens.*"},
			Decode: func(m imps.Message) (any, error) { return m, nil },
			ExtractEntity: func(decoded any) (imps.Entity, error) {
				m := decoded.(imps.Message)
				return imps.Entity(m.Subject[len("crm.opens."):]), nil
			},
		}},
		// Awareness: at most a bounded store call per dispatch (the Request
		// discipline). The update is durable when it returns.
		Awareness: func(ctx context.Context, _ any, entity imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			view, err := customers.Update(ctx, entity, func(st CustomerView) CustomerView {
				st.Opens++
				st.Idle = 0
				return st
			})
			if err != nil {
				log.Printf("persist: %v", err) // errors surface; never a silent zero
				return imps.Ignore()
			}
			if view.Opens%100 == 0 {
				return imps.Think(view, entity)
			}
			return imps.Ignore()
		},
		Thinking: func(ctx context.Context, reason any, entity imps.Entity, t imps.ThinkingContext) error {
			// Thinking uses the store freely; heavier work lives here.
			_, err := customers.Get(ctx, entity)
			return err
		},
	}

	imp, err := imps.NewImp(spec, nc)
	if err != nil {
		log.Fatal(err)
	}

	// Stamp liveness on shutdown so the next start can measure the sleep.
	defer func() {
		stampCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := beacon.Stamp(stampCtx); err != nil {
			log.Printf("beacon stamp: %v", err)
		}
	}()

	if err := imp.Run(ctx); err != nil {
		log.Fatal(err)
	}
	// Stopping IS sleeping: write-through means there is nothing to flush.
}
```

## Driving each behavior deterministically

| Behavior | How to see it | Expected |
|---|---|---|
| Restart survival | Update an entity, stop, restart, access it | State equal under the codec; no replay step |
| Entity wake | Stop for ~1 s, restart, access a previously-active entity | Wake hook fires once with elapsed ≥ the stop; `Idle` advanced before any other code sees the state |
| Bounded residency | Touch more entities than `WithBound` | `Resident()` never exceeds the bound; every entity still reads back correct |
| Never-seen entity | Access a fresh entity | Zero state, no wake, no error |
| Backend failure | Point at a missing bucket / stop the server | `Get`/`Update` return errors — never a silent zero |
| First-ever start | `SleptFor` before any `Stamp` | `ok == false` (absence, not zero sleep) |
| Explicit removal | `Delete(ctx, entity)` | Gone from residency and backend; eviction alone never removes backend state |
