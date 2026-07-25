package soulstream

import (
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream/realm"
	"github.com/impire-io/soulstream/topic"

	imps "github.com/impire-io/imps"
)

// topicChannelConfig accumulates TopicChannelOption effects.
type topicChannelConfig struct {
	name     string
	durable  string
	consumer imps.ConsumerConfig
	decode   imps.Decoder
	extract  imps.EntityExtractor
}

// TopicChannelOption customises a TopicChannel.
type TopicChannelOption func(*topicChannelConfig)

// WithDurable names a durable consumer so a restarted imp resumes from its
// cursor instead of replaying from the baseline. Name uniqueness per imp
// instance is the operator's concern.
func WithDurable(name string) TopicChannelOption {
	return func(c *topicChannelConfig) { c.durable = name }
}

// WithStartSeq starts delivery at a stream sequence (warm rejoin). It
// implies the by-start-sequence deliver policy.
func WithStartSeq(seq uint64) TopicChannelOption {
	return func(c *topicChannelConfig) {
		c.consumer.DeliverPolicy = jetstream.DeliverByStartSequencePolicy
		c.consumer.OptStartSeq = seq
	}
}

// WithStartTime starts delivery at a point in time (warm rejoin). It
// implies the by-start-time deliver policy.
func WithStartTime(t time.Time) TopicChannelOption {
	return func(c *topicChannelConfig) {
		c.consumer.DeliverPolicy = jetstream.DeliverByStartTimePolicy
		c.consumer.OptStartTime = &t
	}
}

// WithDecoder replaces the default header-only decoder. Code using this
// option must not assume the channel yields Op values.
func WithDecoder(d imps.Decoder) TopicChannelOption {
	return func(c *topicChannelConfig) { c.decode = d }
}

// WithEntityExtractor replaces the default extractor (topic path as entity).
func WithEntityExtractor(e imps.EntityExtractor) TopicChannelOption {
	return func(c *topicChannelConfig) { c.extract = e }
}

// WithName overrides the default channel name ("soulstream:"+path).
func WithName(name string) TopicChannelOption {
	return func(c *topicChannelConfig) { c.name = name }
}

// TopicChannel returns a standard harness ChannelSpec that reads the
// topic's op-log: stream SOULSTREAM, filter subject
// SOULSTREAM.TOPICS.OPS.<path> (passed verbatim — no rewriting),
// deliver-all by default, so the imp sees the baseline first, the full
// history in order, then live operations over one continuous consumer.
//
// Declaring the returned spec in ImpSpec.Channels IS joining the topic;
// the harness owns everything after construction, and shutdown deletes an
// ephemeral consumer — which is all "leaving" means. The topic set is
// fixed at Run: this package exposes no runtime join/leave (see the
// package documentation for the registered reversal condition).
func TopicChannel(path string, opts ...TopicChannelOption) imps.ChannelSpec {
	cfg := topicChannelConfig{
		name: defaultChannelName(path),
		consumer: imps.ConsumerConfig{
			DeliverPolicy: jetstream.DeliverAllPolicy,
		},
		decode:  decodeOp,
		extract: extractTopicPath(path),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return imps.ChannelSpec{
		Name: cfg.name,
		Source: imps.StreamSource{
			Stream:         realm.StreamName,
			FilterSubject:  topic.OpsSubject(path),
			Durable:        cfg.durable,
			ConsumerConfig: cfg.consumer,
		},
		Decode:        cfg.decode,
		ExtractEntity: cfg.extract,
	}
}
