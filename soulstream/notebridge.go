package soulstream

import (
	"context"
	"errors"
	"time"

	imps "github.com/impire-io/imps"
)

// Noted is the payload awareness hands the harness's existing Note verdict
// to make a lightweight contribution: a comment anchored to the op it just
// observed.
type Noted struct {
	// AnchorOp is the op-id the note annotates — normally the Op.ID
	// awareness just observed. Required.
	AnchorOp string
	// Body is the comment body. Required.
	Body string
}

// noteTimeout bounds the bridge's synchronous publish so a dead substrate
// cannot hang the dispatch goroutine indefinitely.
const noteTimeout = 10 * time.Second

// NoteBridge returns an OnNote hook for ImpSpec.OnNote. A Noted payload
// becomes a comment.add on the entity's topic (under TopicChannel's
// default extractor the entity IS the topic path), anchored to
// Noted.AnchorOp, authored as the participant's persona, published
// synchronously on the dispatch goroutine with a best-effort frontier —
// legal and merge-safe per the owner's protocol.
//
// Any other payload type is passed to next (dropped when next is nil), so
// soulstream notes compose with local note handling. A Noted with an empty
// AnchorOp or Body, an empty entity, or a publish failure is reported to
// onErr (dropped when onErr is nil); nothing malformed is ever published.
//
// The bridge involves no thinking and adds nothing to awareness's surface:
// it runs in the note hook, outside awareness's hands.
func NoteBridge(p *Participant, next func(imps.Entity, any), onErr func(imps.Entity, Noted, error)) func(imps.Entity, any) {
	if p == nil {
		panic("soulstream: NoteBridge requires a non-nil Participant")
	}
	return func(entity imps.Entity, payload any) {
		noted, ok := payload.(Noted)
		if !ok {
			if next != nil {
				next(entity, payload)
			}
			return
		}
		fail := func(err error) {
			if onErr != nil {
				onErr(entity, noted, err)
			}
		}
		if noted.AnchorOp == "" {
			fail(errors.New("soulstream: Noted.AnchorOp is required"))
			return
		}
		if noted.Body == "" {
			fail(errors.New("soulstream: Noted.Body is required"))
			return
		}
		if entity == "" {
			fail(errors.New("soulstream: note entity is empty — not a topic path"))
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), noteTimeout)
		defer cancel()
		if _, err := p.Topic(string(entity)).AddComment(ctx, noted.Body, noted.AnchorOp); err != nil {
			fail(err)
		}
	}
}
