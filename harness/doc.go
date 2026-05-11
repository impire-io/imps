// Package harness is the public, in-process Go substrate that holds an
// imp together. It exposes a single small surface:
//
//   - ImpSpec: declarative description of an imp (channels, awareness,
//     reasoning, local-state shapes, optional Note hook).
//   - NewImp / Imp.Run / Imp.Shutdown / Imp.Identity / Imp.Ready /
//     Imp.Metrics: the runtime handle and lifecycle.
//   - Verdict: closed sum returned by awareness — Ignore, Note(payload),
//     Wake(reason, entity).
//   - AwarenessContext / ReasoningContext: typed surfaces; the energy
//     gradient is structural — AwarenessContext has no Publish method
//     and no Conn method, so calling awareness.Publish(...) or
//     awareness.Conn() does not compile. ReasoningContext exposes
//     Publish, InFlight, and Conn() *nats.Conn (the escape hatch for
//     generic NATS-based clients).
//   - StateRef: per-entity state slot with Get/Set/Update.
//   - Typed errors: ErrSpecInvalid, ErrCapExceeded, ErrUnknownStateShape,
//     ErrConfigInvalid, ErrStreamNotFound, ErrConsumerIncompatible,
//     ErrSubscriptionFailed.
//
// Channels are either core-NATS subject sources or JetStream stream
// sources. The framework performs no subject transformation — declared
// subjects are wire subjects verbatim (constitution v2.2.0 "Imps see
// one subject path"). Multi-tenant scoping, cross-account routing, and
// subject permissioning are operator concerns (NATS account configuration
// and ACLs), not framework code.
//
// Reasoning runs concurrently — each Wake verdict launches a fresh
// goroutine and dispatch returns immediately.
//
// Capabilities, the soulstream, schedule/KV channels, persistence, and
// audit are explicitly out of scope and ship as separate features.
//
// A worked example is in [examples/echo]. The full developer-facing
// contract lives in [specs/001-harness-core/contracts/public-api.md] and
// the imp-author walkthrough in [specs/001-harness-core/quickstart.md].
package harness
