# Contract: Exported Go API of `github.com/impire-io/imps/soulstream`

The complete exported surface of the nested module. Anything not listed here
is unexported or does not ship in this feature. Signatures are contractual;
doc-comment semantics below each item are part of the contract.

```go
package soulstream // import "github.com/impire-io/imps/soulstream"
```

## Reading a topic

```go
// Op is the imp's header-level view of one topic operation.
type Op struct {
    Type   string // Soulstream-Type header (never filtered by this module)
    Author string // Soulstream-Author header (persona slug; may be empty on malformed ops)
    ID     string // Nats-Msg-Id header (the op-id; the anchor target for notes)
}

// TopicChannel returns a standard harness ChannelSpec that reads the topic's
// op-log: stream "SOULSTREAM", filter "SOULSTREAM.TOPICS.OPS."+path,
// deliver-all by default (baseline first, history then live, one continuous
// consumer). The harness owns the channel after construction; shutdown
// deletes ephemeral consumers (that is "leaving" — the protocol has no
// membership). The topic set is fixed at Run: there is no runtime
// join/leave surface in this module.
func TopicChannel(path string, opts ...TopicChannelOption) imps.ChannelSpec

type TopicChannelOption func(*topicChannelConfig)

// WithDurable names a durable consumer so a restarted imp resumes from its
// cursor instead of replaying from the baseline.
func WithDurable(name string) TopicChannelOption

// WithStartSeq / WithStartTime pass through the harness's existing warm-rejoin
// consumer configuration.
func WithStartSeq(seq uint64) TopicChannelOption
func WithStartTime(t time.Time) TopicChannelOption

// WithDecoder replaces the default header-only decoder. The default yields Op
// and MUST NOT be assumed by code using this option.
func WithDecoder(d imps.Decoder) TopicChannelOption

// WithEntityExtractor replaces the default extractor (which returns the topic
// path as the entity for every op on the channel).
func WithEntityExtractor(e imps.EntityExtractor) TopicChannelOption

// WithName overrides the default channel name ("soulstream:"+path).
func WithName(name string) TopicChannelOption
```

Contractual behavior:

- The returned spec's source is the **existing** `imps.StreamSource`; this
  module introduces no channel kind and performs no subject rewriting — the
  OPS subject is passed verbatim.
- The default decode reads exactly three headers; no payload parsing, no
  topic materialisation, no library calls on the dispatch path.
- Ops of unknown type are delivered like any other (additive vocabulary).
- Startup against a realm without the `SOULSTREAM` stream fails with the
  harness's existing `ErrStreamNotFound` (no new error types for this path).

## Identity and the write path

```go
// Participant is the imp's soulstream identity: the imp's own NATS
// connection wrapped in the owner library's realm client. It never closes
// the wrapped connection.
type Participant struct{ /* unexported */ }

// NewParticipant validates realm/persona slugs and confirms JetStream is
// reachable. persona is required for any write; a Participant constructed
// with an empty persona is read-only (Topic handles refuse to post).
func NewParticipant(ctx context.Context, nc *nats.Conn, realm, persona string, opts ...ParticipantOption) (*Participant, error)

type ParticipantOption func(*participantConfig)

// WithSigner makes every op this participant posts carry an Ed25519
// signature over the owner-defined canonical record.
func WithSigner(key *identity.SigningKey) ParticipantOption

// Topic opens a handle for thinking-tier operations on one topic: PostTurn,
// AddComment, Close, Materialise — all owner-library methods on the returned
// handle. The handle is cheap; open per use.
func (p *Participant) Topic(path string) *topic.Handle

// StartTopic announces a new topic and publishes its initial baseline
// (owner-library StartTopic). Thinking-tier only by convention; awareness
// has no path to a Participant unless the imp author hands it one, which the
// documentation forbids.
func (p *Participant) StartTopic(ctx context.Context, in topic.StartTopicInput) (*topic.Handle, error)
```

Contractual behavior:

- Attribution is single-persona: the owner library's guard (author ≡ persona)
  is surfaced unchanged; posting as another persona returns its error.
- The Participant MUST NOT close, drain, or reconfigure the wrapped
  connection.
- Types from the owner library (`topic.Handle`, `topic.StartTopicInput`,
  `identity.SigningKey`) appear in this API deliberately: the owner's package
  is the write-path contract; this module does not wrap what it cannot
  improve.

## The note bridge

```go
// Noted is the payload awareness hands the existing Note verdict to make a
// lightweight contribution: a comment anchored to the op it just observed.
type Noted struct {
    AnchorOp string // required: op-id the note annotates
    Body     string // required: comment body
}

// NoteBridge returns an OnNote hook. A Noted payload becomes a comment.add
// on the entity's topic (the entity is the topic path under TopicChannel's
// default extractor), authored as the participant's persona, published
// synchronously on the dispatch goroutine with a best-effort frontier. Any
// other payload type is passed to next (dropped when next is nil). A Noted
// with an empty AnchorOp or Body, a non-topic entity, or a publish failure
// is reported to onErr (dropped when onErr is nil); nothing malformed is
// ever published.
func NoteBridge(p *Participant, next func(imps.Entity, any), onErr func(imps.Entity, Noted, error)) func(imps.Entity, any)
```

Contractual behavior:

- Zero thinking involvement: the bridge is invoked by the harness's existing
  note delivery; `ThinksDispatched` is untouched by note flows.
- Synchronous by design (notes are low-rate); the documented upgrade path if
  evidence contradicts this is an internal queue in this module — never a
  harness change.
- The bridge never validates anchors against the topic (no read-before-write
  on the dispatch path); dangling anchors are the reader's concern per the
  owner's protocol.

## Package documentation contract

`doc.go` MUST state: the participation model (declaring a channel is
joining; stopping is leaving), the energy-gradient placement (awareness
observes and notes; thinking contributes), the static-participation boundary
and its registered reversal condition, and the prohibition on handing a
Participant to awareness code.
