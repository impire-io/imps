package harness

import (
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Source is a sealed interface satisfied only by SubjectSource and
// StreamSource. The unexported isSource() method prevents implementation
// outside the harness package.
type Source interface {
	isSource()
}

// SubjectSource declares a core-NATS subject channel. Subject is the
// pre-resolution form; the harness applies the configured subject prefix
// and (in platform mode) the importer account public key segment.
type SubjectSource struct {
	Subject string
}

func (SubjectSource) isSource() {}

// StreamSource declares a JetStream-backed channel. Stream and
// FilterSubject are required. When Durable is empty the harness creates
// (and on shutdown deletes) an ephemeral consumer; when non-empty, the
// harness binds an existing durable consumer (or creates it if absent),
// failing startup with ErrConsumerIncompatible if an existing consumer's
// configuration is incompatible.
type StreamSource struct {
	Stream         string
	FilterSubject  string
	Durable        string
	ConsumerConfig ConsumerConfig
}

func (StreamSource) isSource() {}

// ConsumerConfig exposes the JetStream consumer-config fields the harness
// supports as passthrough. Fields not listed here are not configurable in
// v1; adding a field requires a spec amendment.
//
// AckPolicy defaults to AckExplicitPolicy (the harness requires explicit
// ack to enforce ack-at-awareness-completion timing).
type ConsumerConfig struct {
	AckPolicy     jetstream.AckPolicy
	AckWait       time.Duration
	MaxDeliver    int
	DeliverPolicy jetstream.DeliverPolicy
	ReplayPolicy  jetstream.ReplayPolicy
	OptStartSeq   uint64
	OptStartTime  *time.Time
	MaxAckPending int
	Description   string
}

// Decoder turns a raw inbound message into the typed value awareness
// receives. Errors are recorded; awareness is not invoked for the message.
type Decoder func(msg Message) (any, error)

// EntityExtractor returns the entity for a decoded value. Empty Entity is
// treated as a failure; awareness is not invoked.
type EntityExtractor func(decoded any) (Entity, error)

// Message is the harness's view of an inbound substrate message. Subject
// is the resolved subject the message arrived on. Ack/NAK is owned by the
// harness so user code cannot short-circuit ack timing.
type Message struct {
	Subject string
	Reply   string
	Headers nats.Header
	Data    []byte
}

// ChannelSpec declares one inbound subscription. Source must be exactly
// one of SubjectSource or StreamSource. Decode and ExtractEntity are
// required.
type ChannelSpec struct {
	Name          string
	Source        Source
	Decode        Decoder
	ExtractEntity EntityExtractor
}
