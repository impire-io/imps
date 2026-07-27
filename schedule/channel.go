package schedule

import (
	"time"

	"github.com/nats-io/nats.go/jetstream"

	imps "github.com/impire-io/imps"
)

// channelConfig accumulates ChannelOption effects.
type channelConfig struct {
	name     string
	durable  string
	consumer imps.ConsumerConfig
	decode   imps.Decoder
	extract  imps.EntityExtractor
}

// ChannelOption customises a schedule Channel.
type ChannelOption func(*channelConfig)

// WithDurable names a durable consumer so a restarted imp catches up from
// its cursor — with the server having already expired whatever the
// schedule's TTL says is stale.
func WithDurable(name string) ChannelOption {
	return func(c *channelConfig) { c.durable = name }
}

// WithStartSeq starts delivery at a stream sequence (implies the
// by-start-sequence deliver policy).
func WithStartSeq(seq uint64) ChannelOption {
	return func(c *channelConfig) {
		c.consumer.DeliverPolicy = jetstream.DeliverByStartSequencePolicy
		c.consumer.OptStartSeq = seq
	}
}

// WithStartTime starts delivery at a point in time (implies the
// by-start-time deliver policy).
func WithStartTime(t time.Time) ChannelOption {
	return func(c *channelConfig) {
		c.consumer.DeliverPolicy = jetstream.DeliverByStartTimePolicy
		c.consumer.OptStartTime = &t
	}
}

// WithDecoder replaces the default header-only decoder. Code using this
// option must not assume the channel yields Tick values.
func WithDecoder(d imps.Decoder) ChannelOption {
	return func(c *channelConfig) { c.decode = d }
}

// WithEntityExtractor replaces the default extractor (target subject as
// entity).
func WithEntityExtractor(e imps.EntityExtractor) ChannelOption {
	return func(c *channelConfig) { c.extract = e }
}

// WithName overrides the default channel name ("schedule:"+target).
func WithName(name string) ChannelOption {
	return func(c *channelConfig) { c.name = name }
}

// Channel returns a standard harness ChannelSpec consuming the ticks on
// target from stream: the EXISTING StreamSource — target as the filter
// subject verbatim, deliver-all by default — with the header-only Tick
// decoder and the target subject as the entity. Ticks dispatch like any
// other message; this package introduces no channel kind.
func Channel(stream, target string, opts ...ChannelOption) imps.ChannelSpec {
	cfg := channelConfig{
		name: defaultChannelName(target),
		consumer: imps.ConsumerConfig{
			DeliverPolicy: jetstream.DeliverAllPolicy,
		},
		decode:  decodeTick,
		extract: extractTarget(target),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return imps.ChannelSpec{
		Name: cfg.name,
		Source: imps.StreamSource{
			Stream:         stream,
			FilterSubject:  target,
			Durable:        cfg.durable,
			ConsumerConfig: cfg.consumer,
		},
		Decode:        cfg.decode,
		ExtractEntity: cfg.extract,
	}
}
