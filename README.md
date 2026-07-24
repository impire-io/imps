# imps

A Go framework for building small, focused, awareness-driven agents that run on NATS.

An **imp** is your program. You declare its inbound channels (NATS subjects or JetStream streams), an awareness function (cheap, runs on every message), a thinking function (expensive, runs only when awareness escalates), and per-entity local state. The framework wires up subscriptions, dispatch, goroutine management, and the outbound NATS surface — request/reply, fan-out request/reply, and fire-and-forget publish.

> The boundary between cheap awareness and expensive thinking is **structural**, not a coding convention. Awareness can issue a single request/reply and nothing else; thinking gets the full outbound surface — request/reply, fan-out, fire-and-forget publish, and raw NATS access. Awareness code that tries to fan out, fire-and-forget, or grab the raw connection fails to compile.

## Status

- Core surface — channels, awareness, thinking, local state, request/reply, publish — shipped via PR #1 and PR #2.
- Capabilities, soulstream, sleep/wake, persistence, audit: out of scope here, ship as separate features.

## Install

```bash
go get github.com/impire-io/imps
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

    "github.com/impire-io/imps"
    "github.com/nats-io/nats.go"
)

func main() {
    nc, err := nats.Connect(nats.DefaultURL)
    if err != nil {
        log.Fatalf("nats connect: %v", err)
    }
    defer nc.Drain()

    spec := imps.ImpSpec{
        Name:    "echo",
        Version: "0.1.0",
        Channels: []imps.ChannelSpec{{
            Name:   "inbound",
            Source: imps.SubjectSource{Subject: "messages.in"},
            Decode: func(m imps.Message) (any, error) { return string(m.Data), nil },
            ExtractEntity: func(any) (imps.Entity, error) { return imps.Entity("singleton"), nil },
        }},
        Awareness: func(ctx context.Context, decoded any, entity imps.Entity, a imps.AwarenessContext) imps.Verdict {
            return imps.Think(decoded, entity)
        },
        Thinking: func(ctx context.Context, reason any, _ imps.Entity, r imps.ThinkingContext) error {
            return r.Publish(ctx, "actions.out", []byte(reason.(string)))
        },
    }

    imp, err := imps.NewImp(spec, nc, imps.WithLogger(slog.NewTextHandler(os.Stdout, nil)))
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

| Concept | What it does |
|---|---|
| **Channels** | Inbound NATS subscriptions (core subject or JetStream stream). Decode bytes, extract an entity, dispatch into awareness. |
| **Awareness** | Cheap, synchronous, runs on every message. Returns one of three verdicts: ignore, deliver a note, or escalate to thinking. |
| **Thinking** | Expensive, runs in a fresh goroutine each time awareness escalates. Allowed to publish, fan out, and reach the raw connection. |
| **Memory** | Per-entity local state, bounded by a per-shape cap. Get / set / update. |
| **Action** | Outbound NATS sends — single request/reply, fan-out request/reply, or fire-and-forget publish. |

Subjects are literal — the framework performs no prefix or transformation. Cross-account routing and multi-tenancy are operator concerns (NATS account imports and ACLs).

## Documentation

How the project is run lives in [`hq/`](./hq/README.md) — vision, constitution, working rules, the roadmap, and the numbered journey. Start there for *why* and *how we decide*; start here for *how to use it*.

Design documents (the framework's shape):

- [`hq/00-GENESIS/vision.md`](./hq/00-GENESIS/vision.md) — what an imp is, the energy-gradient principle, what the framework deliberately does *not* do.
- [`hq/02-DESIGN/0001-anatomy.md`](./hq/02-DESIGN/0001-anatomy.md) — the five parts of an imp in detail, with invariants and boundaries.
- [`hq/02-DESIGN/0002-capability-service-pattern.md`](./hq/02-DESIGN/0002-capability-service-pattern.md) — the service-side shape for capabilities the imp reaches over NATS.

Constitutional principles: [`hq/00-GENESIS/constitution.md`](./hq/00-GENESIS/constitution.md) (v2.3.0; `.specify/memory/constitution.md` symlinks to it).

Feature specs, plans, and contracts:

- [`specs/001-harness-core/`](./specs/001-harness-core/) — the core surface. See [`quickstart.md`](./specs/001-harness-core/quickstart.md), [`contracts/public-api.md`](./specs/001-harness-core/contracts/public-api.md), [`contracts/stream-channel.md`](./specs/001-harness-core/contracts/stream-channel.md), [`contracts/observability.md`](./specs/001-harness-core/contracts/observability.md).
- [`specs/002-capability-client/`](./specs/002-capability-client/) — the outbound `Request` / `RequestMany` surface. See [`quickstart.md`](./specs/002-capability-client/quickstart.md), [`contracts/request-reply.md`](./specs/002-capability-client/contracts/request-reply.md).

Package godoc: [`doc.go`](./doc.go).

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
