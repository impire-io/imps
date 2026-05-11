# Quickstart: Talk to NATS from an Imp

This walks through using the outbound NATS surface from an imp's
awareness and reasoning code: a single-shot `Request`, a fan-out
`RequestMany`, and the existing `Publish` from `001-harness-core` — all
on the imp's literal declared subjects (no framework-side
transformation, per constitution v2.2.0 "Imps see one subject path").

This document assumes familiarity with `001-harness-core/quickstart.md`
(the echo imp and the runtime options).

---

## What we're building

A small imp that:

- Subscribes to subject `messages.in`.
- Awareness consults a `classify.short` responder via `a.Request` to
  decide whether to wake.
- Reasoning calls `r.RequestMany` against `health.ping` to survey the
  cluster, then `r.Request` against `knowledge.lookup` for context,
  then `r.Publish` to `actions.out` with a typed result.

Subjects on the wire:

- Channel subscription: `messages.in`
- Single-shot request from awareness: `classify.short`
- Fan-out from reasoning: `health.ping`
- Single-shot request from reasoning: `knowledge.lookup`
- Action publish: `actions.out`

All literal. The framework imposes no prefix or transformation.

If a responder lives in another NATS account, the operator configures
an account *import* that maps the exported subject onto whatever local
name the imp uses (e.g., import the platform-account
`platform.<importer-pk>.knowledge.lookup` and land it on
`knowledge.lookup` in the imp's account). The imp's source — and the
harness — see only `knowledge.lookup`.

---

## Awareness: bounded by call shape

```go
awareness := func(
    ctx context.Context,
    decoded any,
    entity harness.Entity,
    a harness.AwarenessContext,
) harness.Verdict {
    input := decoded.(string)

    // Single round-trip request/reply. Bounded by call shape — one
    // publish, one reply, one effective deadline.
    reply, err := a.Request(ctx, "classify.short", []byte(input),
        harness.WithRequestTimeout(50*time.Millisecond),
    )
    if err != nil {
        // Pattern-match on the two NATS-native categories.
        var noResp *harness.ErrNoResponders
        var toErr  *harness.ErrRequestTimeout
        switch {
        case errors.As(err, &noResp):
            return harness.Note("classifier offline")
        case errors.As(err, &toErr):
            return harness.Note("classifier slow")
        default:
            return harness.Note(err)
        }
    }

    // Use the classifier's reply to choose.
    if string(reply) == "ignore" {
        return harness.Ignore()
    }
    return harness.Wake(reply, entity)
}
```

Notes:

- `Request` is the only outbound method on `AwarenessContext`. The
  awareness code cannot fan out (`RequestMany`), fire-and-forget
  (`Publish`), or reach the raw connection (`Conn()`) — none of those
  methods exist on the interface. Calling any of them fails to compile.
- `WithRequestTimeout(50*time.Millisecond)` overrides the harness's
  default request timeout (5s) for this call. Awareness's dispatch hot
  path wants a tighter bound than reasoning's default.
- The two error categories — `*ErrNoResponders` and
  `*ErrRequestTimeout` — let awareness adapt its verdict to the
  failure mode. Other (rare) errors fall through to the default
  `Note(err)`.
- The framework does not parse the responder's reply. If the responder
  signals an application-level error via JSON payload structure, the
  imp's code interprets it. The framework treats any byte slice as a
  successful return.

---

## Reasoning: full outbound surface

```go
reasoning := func(
    ctx context.Context,
    reason any,
    entity harness.Entity,
    r harness.ReasoningContext,
) error {
    // Survey the cluster via fan-out. Collects every reply within the
    // window, up to 5.
    replies, err := r.RequestMany(ctx, "health.ping", nil,
        harness.WithRequestManyWindow(200*time.Millisecond),
        harness.WithRequestManyMax(5),
    )
    if err != nil {
        return err
    }
    // An empty slice is legitimate — it means no responders were
    // listening, or all of them were too slow. Distinguishing is not
    // the framework's job.
    if len(replies) == 0 {
        return errors.New("cluster offline")
    }

    // Single-shot lookup.
    fact, err := r.Request(ctx, "knowledge.lookup", []byte(entity))
    if err != nil {
        return err
    }

    // Publish — sends on "actions.out" verbatim. Subject permissioning
    // is the substrate's concern (NATS account ACLs); the framework
    // performs no whitelist check.
    return r.Publish(ctx, "actions.out", fact)
}
```

Notes:

- `r.RequestMany` returns `[][]byte` — a slice of reply payloads. Order
  is substrate-determined (whichever responders replied first); the
  harness does not sort.
- `WithRequestManyMax(5)` caps collection. If five replies arrive
  before the window elapses, the call returns immediately. If fewer
  arrive, the call waits the full window.
- An empty `replies` slice is success-with-no-data. There is no
  `*ErrNoResponders` for `RequestMany`'s "nobody home" case (that
  error is reserved for the substrate refusing the publish entirely).
- `r.Publish` is the existing 001 method — byte payload, verbatim
  subject, no framework whitelist.
- `r.Request` against a non-existent subject returns
  `*ErrNoResponders` quickly (the substrate signals this without
  waiting for the timeout).

---

## Generic NATS clients via r.Conn()

`ReasoningContext` exposes `Conn() *nats.Conn` — the escape hatch for
generic NATS-based clients used from reasoning:

```go
reasoning := func(ctx context.Context, _ any, _ harness.Entity, r harness.ReasoningContext) error {
    client := inference.New(r.Conn())     // generic NATS-based client
    resp, err := client.Prompt(ctx, "summarize this")
    if err != nil { return err }
    return r.Publish(ctx, "actions.out", resp)
}
```

`Conn()` is **reasoning-only**. Awareness has no equivalent — the
energy gradient holds structurally.

Caveats:

- `r.Conn().Publish(subject, ...)` and `r.Conn().Request(subject, ...)`
  bypass the framework's metrics counters (`RequestCalls`,
  `RequestManyCalls`, etc.). The framework-method path is the
  observable one.
- All subjects passed to `r.Conn()` methods are also literal — the
  framework has nothing to transform anyway.

---

## Wiring it up

```go
import (
    "context"
    "log"
    "log/slog"
    "os"
    "os/signal"
    "time"

    "github.com/impire-io/imps/harness"
    "github.com/nats-io/nats.go"
)

func main() {
    nc, err := nats.Connect(nats.DefaultURL)
    if err != nil { log.Fatalf("nats connect: %v", err) }
    defer nc.Drain()

    spec := harness.ImpSpec{
        Name:    "request-demo",
        Version: "0.1.0",
        Channels: []harness.ChannelSpec{{
            Name:   "inbound",
            Source: harness.SubjectSource{Subject: "messages.in"},
            Decode: func(msg harness.Message) (any, error) {
                return string(msg.Data), nil
            },
            ExtractEntity: func(any) (harness.Entity, error) {
                return harness.Entity("singleton"), nil
            },
        }},
        Awareness: awareness,
        Reasoning: reasoning,
    }

    imp, err := harness.NewImp(spec, nc,
        harness.WithDefaultRequestTimeout(2*time.Second),
        harness.WithDefaultRequestManyWindow(300*time.Millisecond),
        harness.WithLogger(slog.NewTextHandler(os.Stdout, nil)),
    )
    if err != nil { log.Fatalf("build imp: %v", err) }

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    if err := imp.Run(ctx); err != nil {
        log.Fatalf("run imp: %v", err)
    }
}
```

Note: no `WithSubjectPrefix`, no `WithPlatformMode`, no `Capabilities`
field on the spec, no `Actions` whitelist. The imp's outbound subjects
are named directly; whatever is on the other end is the operator's
design.

---

## Testing — raw `nc.Subscribe` responders

Tests register responders directly in the test body. No special
fixture package is needed.

```go
func TestRequestRoundTrip(t *testing.T) {
    srv := natstest.New(t)
    nc, _ := nats.Connect(srv.URL())
    t.Cleanup(func() { nc.Drain() })

    // A simple echo responder for the classifier subject.
    if _, err := nc.Subscribe("classify.short", func(m *nats.Msg) {
        _ = m.Respond([]byte("wake"))
    }); err != nil { t.Fatal(err) }

    spec := buildSpec()  // as in the sections above
    imp, _ := harness.NewImp(spec, nc)

    ctx, cancel := context.WithCancel(context.Background())
    t.Cleanup(cancel)
    go func() { _ = imp.Run(ctx) }()

    // Wait for the imp to register subscriptions.
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) && !imp.Ready() {
        time.Sleep(10 * time.Millisecond)
    }

    // Drive a message through; the rest of the assertions are the
    // same as the 001-harness-core quickstart pattern.
}
```

Notes:

- The responder is a plain `nc.Subscribe` callback. Nothing
  capability-specific.
- For `RequestMany` tests, register two or three subscribers on the
  same subject (they form a NATS queue group implicitly only if
  configured to; by default they each get every request and each
  reply).
- For `*ErrRequestTimeout` tests, the responder sleeps before
  replying. Pair with `WithRequestTimeout(50ms)`.
- For `*ErrNoResponders` tests, don't subscribe a responder — the
  substrate signals immediately.

---

## Driving each error category deterministically

```go
// 1. no-responders — just don't subscribe a responder.
_, err := r.Request(ctx, "nobody.home", nil)
var noResp *harness.ErrNoResponders
errors.As(err, &noResp)  // true; noResp.Subject == "nobody.home"

// 2. timeout — subscribe a slow responder.
nc.Subscribe("slow", func(m *nats.Msg) {
    time.Sleep(200*time.Millisecond)
    _ = m.Respond([]byte("late"))
})
_, err = r.Request(ctx, "slow", nil, harness.WithRequestTimeout(50*time.Millisecond))
var toErr *harness.ErrRequestTimeout
errors.As(err, &toErr)             // true; toErr.Subject == "slow"
errors.Is(err, context.DeadlineExceeded)  // also true (Unwrap)
```

Each case maps to exactly one error category; nothing collapses to a
generic error.

---

## What the harness did for you

1. Subscribed on the declared subjects (`messages.in`, the channel)
   verbatim.
2. For `Request`: derived a deadline-bounded context, issued
   `nc.RequestWithContext` on the literal subject, translated the
   outcome into one of the two typed error categories or returned
   the reply.
3. For `RequestMany`: created a temporary inbox, subscribed a
   buffered channel, published the request on the literal subject,
   collected replies for the effective window (or up to the cap),
   unsubscribed the inbox on every return path.
4. For `Publish`: called `nc.Publish` on the literal subject — no
   whitelist check, no transformation.
5. Incremented `Metrics.RequestCalls` / `RequestManyCalls` /
   `RequestNoResponders` / `RequestTimeouts` as appropriate.

---

## What's next

The outbound surface is the foundation that future capability-aware
imps (inference, knowledge, tool execution) layer on top of. Each
capability is a NATS responder pattern — the imp's code calls
`r.Request(...)` / `a.Request(...)` and interprets the reply per the
capability's own protocol, or pulls `r.Conn()` and passes it to a
generic capability client library. The framework does not need to
grow to accommodate new capabilities; the framework is done growing
for this purpose.

The capability service pattern (`docs/02-capability-service-pattern.md`)
documents the service-side shape an operator should use when standing
up a capability responder. The imp framework consumes the result with
no special ceremony.
