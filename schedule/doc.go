// Package schedule lets an imp attend periodic work without owning a
// timer: the substrate produces ticks (JetStream message scheduling), and
// the imp consumes them through its ordinary channels.
//
// # The server owns the clock
//
// This package runs no timers, produces no ticks, polls nothing, and holds
// no schedule registry. A schedule is one headered message in a stream
// configured with AllowMsgSchedules; the server appends ticks to the
// schedule's target subject — with Nats-Scheduler provenance — whether any
// imp is running or not. [Channel] is sugar over the harness's existing
// stream source; [Register] and [Deregister] are typed builders over the
// substrate's scheduling headers, defined once here so no imp author
// hand-rolls them.
//
// # Tiers
//
// Registering and deregistering are writes: they belong to thinking or to
// operator tooling, and take a jetstream handle awareness structurally does
// not have. Never hand awareness one. Consuming ticks is ordinary channel
// dispatch — awareness sees a [Tick] like any other decoded message.
//
// # Stale ticks are an explicit choice
//
// [WithTickTTL] makes the server itself expire ticks that outlive the TTL
// (the stream needs AllowMsgTTL): an imp waking from a long gap replays
// only the unexpired tail. OMITTING the TTL is legal and means the full
// backlog accumulates and will all be delivered on wake — choose it
// deliberately (audit trails want it; heartbeats do not).
//
// # Provisioning is the operator's act
//
// The stream — its AllowMsgSchedules and AllowMsgTTL flags and its capture
// of both schedule and tick subjects — is provisioned by the operator; this
// package never creates or reconfigures streams, and an imp declaring a
// tick channel against a missing stream fails startup with the harness's
// named error.
package schedule
