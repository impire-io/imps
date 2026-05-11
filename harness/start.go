package harness

import (
	"context"

	"github.com/nats-io/nats.go"
)

// start establishes every declared subscription. On any failure it returns
// the typed error; the caller (Run) is responsible for invoking unwind to
// roll back any partial subscriptions.
//
// Subject channels are wired here. Stream channels are TBD (US4) and panic
// at startup with ErrSpecInvalid until US4 lands; for the MVP an imp with
// a stream source still validates at construction but cannot Run.
func (i *Imp) start() error {
	for idx := range i.spec.Channels {
		ch := i.spec.Channels[idx]
		switch src := ch.Source.(type) {
		case SubjectSource:
			if err := i.bindSubjectChannel(ch, src); err != nil {
				return err
			}
		case StreamSource:
			if err := i.bindStreamChannel(ch, src); err != nil {
				return err
			}
		default:
			return &ErrSpecInvalid{
				Field:  "ChannelSpec.Source",
				Reason: "unsupported source kind for channel " + ch.Name,
			}
		}
	}
	return nil
}

// bindSubjectChannel registers a core-NATS subscription. The handler runs
// dispatch on a fresh per-message context.
func (i *Imp) bindSubjectChannel(spec ChannelSpec, src SubjectSource) error {
	resolved := i.runtime().resolver.resolve(src.Subject)
	state := &channelState{spec: spec, resolvedSubject: resolved}

	sub, err := i.nc.Subscribe(resolved, func(msg *nats.Msg) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		i.dispatch(ctx, state, Message{
			Subject: msg.Subject,
			Reply:   msg.Reply,
			Headers: msg.Header,
			Data:    msg.Data,
		})
	})
	if err != nil {
		return &ErrSubscriptionFailed{Subject: resolved, Cause: err}
	}
	state.subscription = sub
	i.runtime().channels = append(i.runtime().channels, state)
	i.runtime().logger.info("channel ready",
		"channel", spec.Name,
		"resolved", resolved,
		"kind", "subject",
	)
	return nil
}

// unwind rolls back every subscription established so far. Called both on
// startup failure (mid-establishment) and on shutdown. Idempotent.
func (i *Imp) unwind() {
	rt := i.runtime()
	if rt == nil {
		return
	}
	for _, ch := range rt.channels {
		if ch.subscription != nil {
			if err := ch.subscription.Unsubscribe(); err != nil {
				rt.logger.warn("unsubscribe failed",
					"channel", ch.spec.Name,
					"err", err,
				)
			}
			ch.subscription = nil
		}
	}
	// Stream-channel ephemeral cleanup is hooked in US4; the registered
	// teardown call lives there.
	i.teardownStreamConsumers()
}
