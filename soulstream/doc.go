// Package soulstream lets an imp participate in soulstream topics.
//
// # The participation model
//
// The soulstream protocol has no join, no leave, and no membership state:
// presence is a consumer, and posting rights are subject permissions.
// Declaring a topic with [TopicChannel] in the imp's ImpSpec.Channels IS
// joining — the imp receives the topic's baseline first, then its full
// history, then live operations, through the harness's ordinary dispatch
// path. Stopping the imp IS leaving: an ephemeral channel's consumer is
// deleted on shutdown and nothing lingers on the substrate.
//
// # Placement on the energy gradient
//
// Awareness observes ops cheaply (three decoded headers) and may return the
// harness's existing Note verdict with a [Noted] payload; the [NoteBridge]
// hook turns that into a comment.add contribution anchored to the observed
// op — no thinking involved, and awareness's bounded surface is unchanged.
// Thinking is the full participant: it holds a [Participant] and uses the
// owner library's topic handles to start topics, post turns, add comments,
// close, and materialise views. Never hand a [Participant] to awareness
// code: contribution beyond a note is thinking-tier by design, and the
// compile-enforced awareness boundary cannot see through a captured
// reference.
//
// # Static participation
//
// The topic set is fixed when the imp starts; this package exposes no
// runtime join/leave. That boundary is deliberate (the simpler shape) and
// carries a registered reversal condition — the first real scenario in
// which an imp's topic set must change without a restart — recorded in
// hq/04-JOURNEY/0003-soulstream-participation.md.
//
// The wire protocol is owned by github.com/impire-io/soulstream; this
// package consumes it through the owner's realm and topic packages and
// defines none of it.
package soulstream
