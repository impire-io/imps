# imps

A Go framework for building small, focused, awareness-driven agents that run on NATS.

An **imp** watches one slice of the world, interprets cheaply on every message, and reasons only when the interpretation crosses a threshold worth acting on. The **harness** in this repo is the in-process Go substrate that holds an imp together: channels in, awareness, reasoning, local per-entity state, action out.

> The boundary between cheap awareness and expensive reasoning is **structural**, not a coding convention. `AwarenessContext` exposes only `Request`; `ReasoningContext` exposes `Request`, `RequestMany`, `Publish`, and `Conn()`. Awareness code that tries to fan out, fire-and-forget, or grab the raw NATS connection fails to compile.

## Status

- `harness` package: in-process Go substrate (channels, awareness, reasoning, local state, request/reply, publish). Shipped via PR #1 and PR #2.
- Capabilities, soulstream, sleep/wake, persistence, audit: out of scope here, ship as separate features.

## Install

```bash
go get github.com/impire-io/imps/harness
```

Requires Go 1.25+. The only runtime dependencies are `nats.go` and (for the embedded test server) `nats-server/v2`.

## Quickstart

A minimal echo imp — subscribes to `messages.in`, republishes to `actions.out`:

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
            Decode: func(m harness.Message) (any, error) { return string(m.Data), nil },
            ExtractEntity: func(any) (harness.Entity, error) { return harness.Entity("singleton"), nil },
        }},
        Awareness: func(ctx context.Context, decoded any, entity harness.Entity, a harness.AwarenessContext) harness.Verdict {
            return harness.Wake(decoded, entity)
        },
        Reasoning: func(ctx context.Context, reason any, _ harness.Entity, r harness.ReasoningContext) error {
            return r.Publish(ctx, "actions.out", []byte(reason.(string)))
        },
    }

    imp, err := harness.NewImp(spec, nc, harness.WithLogger(slog.NewTextHandler(os.Stdout, nil)))
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

Verify with a local NATS server:

```bash
nats sub 'actions.out'        # terminal 1
nats pub 'messages.in' hello  # terminal 2 — terminal 1 prints "hello"
```

A complete runnable version (with embedded-server test) lives in [`examples/echo`](./examples/echo).

## The five concepts

| Concept | What it does | Surface |
|---|---|---|
| **Channels** | Inbound NATS subscriptions (core subject or JetStream stream). Decode bytes, extract an entity, dispatch into awareness. | `ChannelSpec`, `SubjectSource`, `StreamSource` |
| **Awareness** | Cheap, synchronous, runs on every message. Returns `Ignore`, `Note(payload)`, or `Wake(reason, entity)`. | `AwarenessContext`: `State`, `Request` |
| **Reasoning** | Expensive, runs in a fresh goroutine per `Wake`. Allowed to publish, fan out, and reach the raw connection. | `ReasoningContext`: `State`, `Publish`, `Request`, `RequestMany`, `Conn`, `InFlight` |
| **Memory** | Per-entity local state, bounded by `Cap`. `Get` / `Set` / `Update`. | `StateShape`, `StateRef` |
| **Action** | Outbound NATS sends — single request/reply, fan-out request/reply, or fire-and-forget publish. | `r.Request`, `r.RequestMany`, `r.Publish` |

Subjects are literal — the framework performs no prefix or transformation. Cross-account routing and multi-tenancy are operator concerns (NATS account imports and ACLs).

## Documentation

Design documents (start here to understand the framework's shape):

- [`docs/00-vision.md`](./docs/00-vision.md) — what an imp is, the energy-gradient principle, what the framework deliberately does *not* do.
- [`docs/01-harness-anatomy.md`](./docs/01-harness-anatomy.md) — the five concepts in detail, with invariants and boundaries.
- [`docs/02-capability-service-pattern.md`](./docs/02-capability-service-pattern.md) — the service-side shape for capabilities the imp reaches over NATS.

Constitutional principles: [`.specify/memory/constitution.md`](./.specify/memory/constitution.md) (v2.2.0).

Feature specs, plans, and contracts:

- [`specs/001-harness-core/`](./specs/001-harness-core/) — the substrate. See [`quickstart.md`](./specs/001-harness-core/quickstart.md), [`contracts/public-api.md`](./specs/001-harness-core/contracts/public-api.md), [`contracts/stream-channel.md`](./specs/001-harness-core/contracts/stream-channel.md), [`contracts/observability.md`](./specs/001-harness-core/contracts/observability.md).
- [`specs/002-capability-client/`](./specs/002-capability-client/) — the outbound `Request` / `RequestMany` surface. See [`quickstart.md`](./specs/002-capability-client/quickstart.md), [`contracts/request-reply.md`](./specs/002-capability-client/contracts/request-reply.md).

Package godoc: [`harness/doc.go`](./harness/doc.go).

## Development

```bash
make fmt       # gofmt + goimports
make test      # race detector + compile-deny invariants
make lint      # golangci-lint
make check     # fmt + tidy + test + lint
```

The `compile-deny` step asserts that three build tags — `awareness_publish_must_fail`, `awareness_requestmany_must_fail`, `awareness_conn_must_fail` — each fail to compile, proving `Publish`, `RequestMany`, and `Conn` are structurally absent from `AwarenessContext`. A successful build under any of these tags is a regression.

## License

TBD.
