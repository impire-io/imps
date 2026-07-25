package soulstream

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream/identity"
	"github.com/impire-io/soulstream/realm"
	"github.com/impire-io/soulstream/topic"
)

// Participant is the imp's soulstream identity: the imp's own NATS
// connection wrapped in the owner library's realm client. It never closes
// the wrapped connection — the imp owns it.
type Participant struct {
	client *realm.Client
}

// participantConfig accumulates ParticipantOption effects.
type participantConfig struct {
	signer *identity.SigningKey
}

// ParticipantOption customises a Participant.
type ParticipantOption func(*participantConfig)

// WithSigner makes every op this participant posts carry an Ed25519
// signature over the owner-defined canonical record.
func WithSigner(key *identity.SigningKey) ParticipantOption {
	return func(c *participantConfig) { c.signer = key }
}

// NewParticipant validates the realm and persona slugs and confirms
// JetStream is reachable on the given connection. persona is required for
// any write; a Participant constructed with an empty persona is read-only
// (topic handles refuse to post).
//
// The connection is never closed by the Participant. The owner library's
// client closes a connection it fails to finish constructing around, so
// JetStream reachability is confirmed here first — a NewParticipant error
// leaves nc open.
func NewParticipant(ctx context.Context, nc *nats.Conn, realmName, persona string, opts ...ParticipantOption) (*Participant, error) {
	var cfg participantConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// Reachability pre-check on our side of the boundary, so a failure
	// surfaces before the owner library takes (and on error closes) the
	// connection.
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("soulstream: initialise jetstream: %w", err)
	}
	if _, err := js.AccountInfo(ctx); err != nil {
		return nil, fmt.Errorf("soulstream: jetstream unavailable: %w", err)
	}

	client, err := realm.NewClient(ctx, nc, realm.Config{
		Realm:   realmName,
		Persona: persona,
		Signer:  cfg.signer,
	})
	if err != nil {
		return nil, fmt.Errorf("soulstream: realm client: %w", err)
	}
	return &Participant{client: client}, nil
}

// Topic opens a handle for thinking-tier operations on one topic:
// PostTurn, AddComment, Close, Materialise — the owner library's methods
// on the returned handle. The handle is cheap; open per use.
func (p *Participant) Topic(path string) *topic.Handle {
	return topic.Open(p.client, path)
}

// StartTopic announces a new topic and publishes its initial baseline via
// the owner library. Thinking-tier only by convention: never hand a
// Participant to awareness code (see the package documentation).
func (p *Participant) StartTopic(ctx context.Context, in topic.StartTopicInput) (*topic.Handle, error) {
	return topic.StartTopic(ctx, p.client, in)
}
