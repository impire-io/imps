// Package imps is the Go framework for building an imp. It exposes a
// single small surface:
//
//   - ImpSpec: declarative description of an imp (channels, awareness,
//     thinking, local-state shapes, optional Note hook).
//   - NewImp / Imp.Run / Imp.Shutdown / Imp.Identity / Imp.Ready /
//     Imp.Metrics: the runtime handle and lifecycle.
//   - Verdict: closed sum returned by awareness — Ignore, Note(payload),
//     Think(reason, entity).
//   - AwarenessContext / ThinkingContext: typed surfaces; the energy
//     gradient is structural — AwarenessContext exposes State and
//     Request only, so calling awareness.RequestMany(...), Publish(...),
//     or Conn() does not compile. ThinkingContext exposes State,
//     Publish, InFlight, Conn() *nats.Conn (the escape hatch for
//     generic NATS-based clients), Request, and RequestMany.
//   - RequestOption / RequestManyOption: per-call functional options;
//     WithRequestTimeout, WithRequestManyWindow, WithRequestManyMax.
//   - WithDefaultRequestTimeout / WithDefaultRequestManyWindow:
//     construction options for the request defaults (5s / 1s).
//   - StateRef: per-entity state slot with Get/Set/Update.
//   - Typed errors: ErrSpecInvalid, ErrConfigInvalid, ErrStreamNotFound,
//     ErrConsumerIncompatible, ErrSubscriptionFailed, ErrNoResponders,
//     ErrRequestTimeout (the latter unwraps to context.DeadlineExceeded).
//
// Channels are either core-NATS subject sources or JetStream stream
// sources. The framework performs no subject transformation — declared
// subjects are wire subjects verbatim (constitution v2.2.0 "Imps see
// one subject path"). Multi-tenant scoping, cross-account routing, and
// subject permissioning are operator concerns (NATS account configuration
// and ACLs), not framework code.
//
// Thinking runs concurrently — each Think verdict launches a fresh
// goroutine and dispatch returns immediately.
//
// The outbound NATS surface is byte-shaped — Request and RequestMany
// take and return []byte. No codec is imposed by the framework. The
// framework performs no retry, no backoff, and no circuit-breaker: a
// timeout failure does not produce a second NATS publish on the same
// subject.
//
// Capabilities, the soulstream, schedule/KV channels, persistence, and
// audit are explicitly out of scope and ship as separate features.
//
// A worked example is in [examples/echo]. The full developer-facing
// contract lives in [specs/001-harness-core/contracts/public-api.md] and
// the imp-author walkthrough in [specs/001-harness-core/quickstart.md].
package imps
