# Quickstart: Author and Run an Imp

This walks through building the smallest meaningful imp using the harness — one channel, one awareness function, one reasoning function, one outbound publish — and running it against an embedded NATS server. Everything below is what an imp author writes; the harness fills in subscription, dispatch, queueing, and lifecycle.

---

## What we're building

An "echo" imp:
- Subscribes to subject `messages.in`.
- Awareness sees every message and always returns `Wake` (this is the simplest path; a real awareness would inspect content first).
- Reasoning publishes the message back out to subject `actions.out`.

The framework imposes no subject transformation — `messages.in` and `actions.out` are the literal subjects on the wire (constitution v2.2.0 "Imps see one subject path").

---

## Code

```go
package main

import (
    "context"
    "log"
    "log/slog"
    "os"
    "os/signal"

    "github.com/impire-io/imps/harness"
    "github.com/nats-io/nats.go"
)

func main() {
    nc, err := nats.Connect(nats.DefaultURL)
    if err != nil {
        log.Fatalf("nats connect: %v", err)
    }
    defer nc.Drain()

    spec := harness.ImpSpec{
        Name:    "echo",
        Version: "0.1.0",
        Channels: []harness.ChannelSpec{{
            Name:   "inbound",
            Source: harness.SubjectSource{Subject: "messages.in"},
            Decode: func(msg harness.Message) (any, error) {
                return string(msg.Data), nil
            },
            ExtractEntity: func(decoded any) (harness.Entity, error) {
                return harness.Entity("singleton"), nil
            },
        }},
        Awareness: func(
            ctx context.Context,
            decoded any,
            entity harness.Entity,
            a harness.AwarenessContext,
        ) harness.Verdict {
            return harness.Wake(decoded, entity)
        },
        Reasoning: func(
            ctx context.Context,
            reason any,
            entity harness.Entity,
            r harness.ReasoningContext,
        ) error {
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
```

That's an entire imp. ~50 lines of declaration, no subscription code, no dispatch loop, no goroutine management, no subject-prefix ceremony.

---

## Verify it works

In a second terminal, with `nats-server` running locally:

```bash
nats sub 'actions.out'
```

In a third terminal:

```bash
nats pub 'messages.in' 'hello'
```

The subscriber prints `hello`. The imp received the message on `messages.in`, awareness returned `Wake`, reasoning published to `actions.out`, and the subscriber saw the result.

---

## What the harness did for you

1. Validated the spec at `NewImp` (would have rejected a missing awareness function, duplicate state names, non-positive caps, etc.).
2. Established the NATS subscription on `messages.in` (verbatim).
3. On every message: invoked the decoder, then the entity extractor, then awareness — all synchronously.
4. On `Wake`: returned to the substrate (the message is now "handled"), and queued reasoning in a fresh goroutine.
5. In the reasoning goroutine: published on `actions.out` (verbatim) via the NATS connection.
6. On `SIGINT`: cancelled the context, stopped accepting messages, waited up to 30 s for any in-flight reasoning to drain, unsubscribed cleanly.

---

## Reaching a service in another account

Per the constitution's "Imps see one subject path" principle (v2.2.0), the imp's view of NATS subjects is the same single form regardless of where responders live. If a responder lives in a different NATS account, the operator configures an account *import* on the imp's account so the exported subject lands on whatever name the imp uses internally. The imp's source — and the harness — sees `knowledge.recall`; the substrate routes that through to the exporting account.

No `WithPlatformMode`, no `WithSubjectPrefix`. The imp source and the harness API are topology-agnostic; cross-account routing is operator concern.

---

## Generic NATS clients in reasoning

The `ReasoningContext` exposes `Conn() *nats.Conn`. Any downstream library that takes a `*nats.Conn` (an inference client, a knowledge client, a tool client) can be used from reasoning directly:

```go
spec.Reasoning = func(ctx context.Context, reason any, _ harness.Entity, r harness.ReasoningContext) error {
    client := inference.New(r.Conn())   // generic NATS-based client
    return client.Prompt(ctx, "...")
}
```

Awareness has no `Conn()` method. A library that needs raw connection access cannot be invoked from awareness — the structural energy gradient holds.

---

## Adding per-entity local memory

To track a counter per entity:

```go
type counter struct{ n int }

spec.States = []harness.StateShape{{
    Name:    "counter",
    Factory: func() any { return &counter{} },
    Cap:     1000,
}}

spec.Awareness = func(
    ctx context.Context,
    decoded any,
    entity harness.Entity,
    a harness.AwarenessContext,
) harness.Verdict {
    ref, err := a.State("counter", entity)
    if err != nil {
        return harness.Note(err)
    }
    if err := ref.Update(func(v any) any {
        c := v.(*counter)
        c.n++
        return c
    }); err != nil {
        return harness.Note(err)
    }
    if v, _ := a.State("counter", entity); v.Get().(*counter).n%10 == 0 {
        return harness.Wake("milestone", entity)
    }
    return harness.Ignore()
}
```

Cap-exceeded errors come through `a.State(...)` when the 1001st distinct entity arrives — the call returns `harness.ErrCapExceeded{Shape: "counter", Count: 1000}`. Existing entities continue to work.

---

## Adding a stream channel

To consume from a JetStream stream rather than a raw subject:

```go
spec.Channels = []harness.ChannelSpec{{
    Name: "orders",
    Source: harness.StreamSource{
        Stream:        "ORDERS",
        FilterSubject: "orders.created",  // literal — must match a subject the stream covers
        Durable:       "echo-orders",     // omit for ephemeral
    },
    Decode:        decodeOrder,
    ExtractEntity: extractOrderID,
}}
```

The dispatch contract is identical to a subject channel. The harness binds (or creates) consumer `echo-orders` on stream `ORDERS` at startup, and the substrate's redelivery semantics apply on decode/extraction/awareness failure (NAK + max-deliveries policy).

---

## Testing your imp

The integration-test pattern, mirrored across User Story 1's Independent Test:

```go
package main_test

import (
    "context"
    "testing"
    "time"

    "github.com/impire-io/imps/harness"
    "github.com/impire-io/imps/testutil/natstest"
    "github.com/nats-io/nats.go"
)

func TestEchoEndToEnd(t *testing.T) {
    srv := natstest.New(t)
    nc, _ := nats.Connect(srv.URL())
    t.Cleanup(func() { nc.Drain() })

    spec := buildEchoSpec()
    imp, err := harness.NewImp(spec, nc)
    if err != nil { t.Fatal(err) }

    ctx, cancel := context.WithCancel(context.Background())
    t.Cleanup(cancel)
    go imp.Run(ctx)

    // Wait for startup to register subscriptions.
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) && !imp.Ready() {
        time.Sleep(10 * time.Millisecond)
    }

    received := make(chan []byte, 1)
    nc.Subscribe("actions.out", func(m *nats.Msg) { received <- m.Data })
    nc.Flush()

    if err := nc.Publish("messages.in", []byte("hello")); err != nil {
        t.Fatal(err)
    }

    select {
    case data := <-received:
        if string(data) != "hello" { t.Fatalf("got %q", data) }
    case <-time.After(time.Second):
        t.Fatal("timeout waiting for action")
    }
}
```

That's the SC-002 test (under 1 second per round-trip). The pattern generalizes to all the spec's User Story Independent Tests.

---

## What's next

Once you have an echo imp running, the same shape extends:

- More channels (subject and/or stream) — declare more entries in `Channels`.
- More state shapes — declare more entries in `States`. Caps fail loudly when reached.
- More outbound publishes — call `r.Publish(...)` from reasoning on any subject the substrate's ACLs permit. (No framework-side whitelist.)
- Generic NATS-based clients — pull `r.Conn()` and pass it to any library that takes a `*nats.Conn`.

Capability calls (inference, knowledge, tools), soulstream participation, sleep/wake, and persistence are separate features — they layer onto this shape without changing it.
