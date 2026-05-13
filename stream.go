package imps

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// streamStartupTimeout bounds the per-call deadline for stream/consumer
// metadata operations (lookup, create, info). Short on purpose — startup
// failures should surface quickly so Run can roll back cleanly.
const streamStartupTimeout = 10 * time.Second

// bindStreamChannel implements the JetStream-backed channel path. The
// per-step error handling matches contracts/stream-channel.md "Startup
// behavior":
//  1. lookup stream → ErrStreamNotFound on miss
//  2. resolve consumer (bind durable / create durable / create ephemeral
//     under generated name) → ErrConsumerIncompatible if the existing
//     durable's config differs from declared on the compared fields
//  3. start the consume loop using JetStream's push-style callback over a
//     pull consumer; per-message Decode → Extract → Awareness → ACK
//     happens in dispatchStream.
func (i *Imp) bindStreamChannel(spec ChannelSpec, src StreamSource) error {
	js, err := i.ensureJetStream()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), streamStartupTimeout)
	defer cancel()

	stream, err := js.Stream(ctx, src.Stream)
	if err != nil {
		if errors.Is(err, jetstream.ErrStreamNotFound) {
			return &ErrStreamNotFound{Stream: src.Stream}
		}
		return err
	}

	desired := buildConsumerConfig(src, src.FilterSubject)

	consumer, consumerName, ephemeral, err := i.resolveConsumer(ctx, stream, src, desired)
	if err != nil {
		return err
	}

	state := &channelState{
		spec:         spec,
		subject:      src.FilterSubject,
		consumerName: consumerName,
		stream:       src.Stream,
		ephemeral:    ephemeral,
	}

	cc, err := consumer.Consume(func(msg jetstream.Msg) {
		i.dispatchStream(state, msg)
	})
	if err != nil {
		// Roll back the consumer we just created if we fail to start consuming.
		if ephemeral {
			delCtx, delCancel := context.WithTimeout(context.Background(), streamStartupTimeout)
			_ = stream.DeleteConsumer(delCtx, consumerName)
			delCancel()
		}
		return &ErrSubscriptionFailed{Subject: src.FilterSubject, Cause: err}
	}
	state.streamConsume = cc
	i.runtime().channels = append(i.runtime().channels, state)
	i.runtime().streams = append(i.runtime().streams, stream)
	i.runtime().logger.info("channel ready",
		"channel", spec.Name,
		"subject", src.FilterSubject,
		"kind", "stream",
		"stream", src.Stream,
		"consumer", consumerName,
		"ephemeral", ephemeral,
	)
	return nil
}

// ensureJetStream creates the JetStream context lazily — only when the
// first stream channel is bound. Subject-only imps never pay the JS init
// cost.
func (i *Imp) ensureJetStream() (jetstream.JetStream, error) {
	if i.runtime().js != nil {
		return i.runtime().js, nil
	}
	js, err := jetstream.New(i.nc)
	if err != nil {
		return nil, err
	}
	i.runtime().js = js
	return js, nil
}

// resolveConsumer implements the ephemeral-vs-durable branch and the
// compatibility check on existing durable consumers. Returns the
// consumer, its server-known name, and whether it is ephemeral (used
// during shutdown to decide whether to delete it).
func (i *Imp) resolveConsumer(
	ctx context.Context,
	stream jetstream.Stream,
	src StreamSource,
	desired jetstream.ConsumerConfig,
) (jetstream.Consumer, string, bool, error) {
	if src.Durable == "" {
		// Ephemeral: leave Durable empty; the server generates a name.
		cfg := desired
		cfg.Durable = ""
		consumer, err := stream.CreateConsumer(ctx, cfg)
		if err != nil {
			return nil, "", false, &ErrSubscriptionFailed{
				Subject: desired.FilterSubject,
				Cause:   err,
			}
		}
		name := consumer.CachedInfo().Name
		return consumer, name, true, nil
	}

	// Durable name supplied. Look up the consumer; create if absent;
	// compatibility-check if present.
	desired.Durable = src.Durable
	desired.Name = src.Durable

	existing, err := stream.Consumer(ctx, src.Durable)
	if err != nil {
		if errors.Is(err, jetstream.ErrConsumerNotFound) {
			consumer, cerr := stream.CreateConsumer(ctx, desired)
			if cerr != nil {
				return nil, "", false, &ErrSubscriptionFailed{
					Subject: desired.FilterSubject,
					Cause:   cerr,
				}
			}
			return consumer, src.Durable, false, nil
		}
		return nil, "", false, err
	}
	if diff := compatDiff(existing.CachedInfo(), desired); diff != "" {
		return nil, "", false, &ErrConsumerIncompatible{
			Consumer: src.Durable,
			Diff:     diff,
		}
	}
	return existing, src.Durable, false, nil
}

// buildConsumerConfig converts the harness ConsumerConfig + source into
// the jetstream ConsumerConfig the server consumes. Defaults applied:
// AckExplicit (the harness requires explicit ack to enforce ack timing).
func buildConsumerConfig(src StreamSource, resolvedFilter string) jetstream.ConsumerConfig {
	cfg := jetstream.ConsumerConfig{
		FilterSubject: resolvedFilter,
		AckPolicy:     jetstream.AckExplicitPolicy,
	}
	cc := src.ConsumerConfig
	if cc.AckPolicy != 0 {
		cfg.AckPolicy = cc.AckPolicy
	}
	if cc.AckWait > 0 {
		cfg.AckWait = cc.AckWait
	}
	if cc.MaxDeliver != 0 {
		cfg.MaxDeliver = cc.MaxDeliver
	}
	if cc.DeliverPolicy != 0 {
		cfg.DeliverPolicy = cc.DeliverPolicy
	}
	if cc.ReplayPolicy != 0 {
		cfg.ReplayPolicy = cc.ReplayPolicy
	}
	if cc.OptStartSeq != 0 {
		cfg.OptStartSeq = cc.OptStartSeq
	}
	if cc.OptStartTime != nil {
		cfg.OptStartTime = cc.OptStartTime
	}
	if cc.MaxAckPending != 0 {
		cfg.MaxAckPending = cc.MaxAckPending
	}
	if cc.Description != "" {
		cfg.Description = cc.Description
	}
	return cfg
}

// compatDiff implements the compatibility check from
// contracts/stream-channel.md "Compatibility check". Returns an empty
// string when compatible, or a human-readable diff naming the offending
// fields. The check is intentionally narrow.
func compatDiff(existing *jetstream.ConsumerInfo, desired jetstream.ConsumerConfig) string {
	if existing == nil {
		return "existing consumer info unavailable"
	}
	got := existing.Config
	var diffs []string

	if got.FilterSubject != desired.FilterSubject {
		diffs = append(diffs, formatDiff("filter_subject", got.FilterSubject, desired.FilterSubject))
	}
	if got.AckPolicy != jetstream.AckExplicitPolicy {
		diffs = append(diffs, formatDiff("ack_policy", got.AckPolicy.String(), "explicit"))
	}
	if desired.DeliverPolicy != 0 && got.DeliverPolicy != desired.DeliverPolicy {
		diffs = append(diffs, formatDiff("deliver_policy", got.DeliverPolicy.String(), desired.DeliverPolicy.String()))
	}
	if desired.ReplayPolicy != 0 && got.ReplayPolicy != desired.ReplayPolicy {
		diffs = append(diffs, formatDiff("replay_policy", got.ReplayPolicy.String(), desired.ReplayPolicy.String()))
	}
	if desired.AckWait > 0 && got.AckWait < desired.AckWait {
		diffs = append(diffs, formatDiff("ack_wait", got.AckWait.String(), desired.AckWait.String()))
	}
	if desired.MaxDeliver != 0 && got.MaxDeliver != desired.MaxDeliver {
		diffs = append(diffs, formatDiffInt("max_deliver", got.MaxDeliver, desired.MaxDeliver))
	}
	return strings.Join(diffs, "; ")
}

func formatDiff(field, existing, declared string) string {
	return field + `: existing="` + existing + `", declared="` + declared + `"`
}

func formatDiffInt(field string, existing, declared int) string {
	return field + ": existing=" + itoa(existing) + ", declared=" + itoa(declared)
}

func itoa(i int) string {
	// Avoid pulling strconv just for this; small helper keeps the import
	// list tight.
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// dispatchStream is the per-message JetStream pipeline. It mirrors the
// step list in contracts/stream-channel.md "Per-message dispatch and ack
// timing": Decode → Extract → Awareness → ACK → branch on verdict, with
// NAK on any pre-ack failure.
func (i *Imp) dispatchStream(ch *channelState, msg jetstream.Msg) {
	hMsg := Message{
		Subject: msg.Subject(),
		Reply:   msg.Reply(),
		Headers: msg.Headers(),
		Data:    msg.Data(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	decoded, err := ch.spec.Decode(hMsg)
	if err != nil {
		i.runtime().metrics.DecodeFailures.Add(1)
		i.runtime().metrics.NakTotal.Add(1)
		i.nakAndLog(msg, "decode failure", ch.spec.Name, err)
		return
	}

	entity, err := ch.spec.ExtractEntity(decoded)
	if err != nil || entity == "" {
		i.runtime().metrics.ExtractionFailures.Add(1)
		i.runtime().metrics.NakTotal.Add(1)
		i.nakAndLog(msg, "extraction failure", ch.spec.Name, err)
		return
	}

	verdict, panicked := i.safeAwareness(ctx, decoded, entity)
	if panicked {
		i.runtime().metrics.AwarenessPanics.Add(1)
		i.runtime().metrics.NakTotal.Add(1)
		_ = msg.Nak()
		return
	}

	// ACK happens here, regardless of verdict (FR-008a, clarification Q2).
	if err := msg.Ack(); err != nil {
		i.runtime().logger.warn("ack failed",
			"channel", ch.spec.Name,
			"err", err,
		)
		// substrate redelivery semantics govern; do not double-ack.
	}

	switch verdict.kind {
	case verdictIgnore:
		i.runtime().metrics.IgnoredVerdicts.Add(1)
	case verdictNote:
		i.runtime().metrics.NotesDelivered.Add(1)
		i.invokeNote(entity, verdict.payload)
	case verdictThink:
		i.runtime().metrics.ThinksDispatched.Add(1)
		i.launchThinking(verdict.reason, verdict.entity)
	}
}

func (i *Imp) nakAndLog(msg jetstream.Msg, event, channelName string, cause error) {
	if err := msg.Nak(); err != nil {
		i.runtime().logger.warn("nak failed",
			"channel", channelName,
			"event", event,
			"err", err,
		)
	}
	i.runtime().logger.warn(event,
		"channel", channelName,
		"err", cause,
	)
}

// teardownStreamConsumers stops every active stream-channel consume loop
// and deletes ephemeral consumers. Called from unwind on shutdown or
// startup failure. Errors are logged and do not block teardown — the
// substrate's own consumer GC will eventually clean up if delete fails.
func (i *Imp) teardownStreamConsumers() {
	rt := i.runtime()
	if rt == nil || rt.js == nil {
		return
	}
	for _, ch := range rt.channels {
		if ch.streamConsume != nil {
			ch.streamConsume.Stop()
			ch.streamConsume = nil
		}
		if ch.ephemeral && ch.consumerName != "" && ch.stream != "" {
			ctx, cancel := context.WithTimeout(context.Background(), streamStartupTimeout)
			s, err := i.runtime().js.Stream(ctx, ch.stream)
			if err == nil {
				if delErr := s.DeleteConsumer(ctx, ch.consumerName); delErr != nil {
					i.runtime().logger.warn("ephemeral consumer delete failed",
						"channel", ch.spec.Name,
						"consumer", ch.consumerName,
						"err", delErr,
					)
				} else {
					i.runtime().logger.info("ephemeral consumer deleted",
						"channel", ch.spec.Name,
						"consumer", ch.consumerName,
					)
				}
			}
			cancel()
			ch.consumerName = ""
		}
	}
}
