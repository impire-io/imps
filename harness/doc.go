// Package harness is the public, in-process Go substrate that holds an
// imp together. It exposes a single small surface:
//
//   - ImpSpec: declarative description of an imp (channels, awareness,
//     reasoning, local-state shapes, action whitelist, optional Note
//     hook).
//   - NewImp / Imp.Run / Imp.Shutdown / Imp.Identity / Imp.Metrics:
//     the runtime handle and lifecycle.
//   - Verdict: closed sum returned by awareness — Ignore, Note(payload),
//     Wake(reason, entity).
//   - AwarenessContext / ReasoningContext: typed surfaces; the energy
//     gradient is structural — AwarenessContext has no Publish method, so
//     calling awareness.Publish(...) does not compile.
//   - StateRef: per-entity state slot with Get/Set/Update.
//   - Typed errors: ErrSpecInvalid, ErrCapExceeded, ErrUnknownStateShape,
//     ErrWhitelistViolation, ErrConfigInvalid, ErrStreamNotFound,
//     ErrConsumerIncompatible, ErrSubscriptionFailed.
//
// Channels are either core-NATS subject sources or JetStream stream
// sources. Reasoning runs concurrently — each Wake verdict launches a
// fresh goroutine and dispatch returns immediately. The action publish
// path checks the imp's declared Actions whitelist before any substrate
// call.
//
// Capabilities, the soulstream, schedule/KV channels, persistence, and
// audit are explicitly out of scope and ship as separate features.
//
// A worked example is in [examples/echo]. The full developer-facing
// contract lives in [specs/001-harness-core/contracts/public-api.md] and
// the imp-author walkthrough in [specs/001-harness-core/quickstart.md].
package harness
