package soulstream

import (
	"github.com/impire-io/soulstream/record"

	imps "github.com/impire-io/imps"
)

// Op is the imp's header-level view of one topic operation. It is produced
// by TopicChannel's default decoder from headers alone — no payload parse,
// no materialisation — so the dispatch path stays cheap.
type Op struct {
	// Type is the operation type (e.g. "baseline", "turn.post",
	// "comment.add"). This package never filters by type: the vocabulary is
	// additive, and unknown types flow to awareness like any other.
	Type string
	// Author is the posting persona's slug. May be empty on a malformed op;
	// delivery still happens — awareness judges.
	Author string
	// ID is the op-id, the value a Noted anchors to.
	ID string
}

// decodeOp is TopicChannel's default Decoder.
func decodeOp(m imps.Message) (any, error) {
	return Op{
		Type:   m.Headers.Get(record.HeaderType),
		Author: m.Headers.Get(record.HeaderAuthor),
		ID:     m.Headers.Get(record.HeaderMsgID),
	}, nil
}

// extractTopicPath is TopicChannel's default EntityExtractor: every op on
// the channel belongs to the topic, so the topic path is the entity.
func extractTopicPath(path string) imps.EntityExtractor {
	return func(any) (imps.Entity, error) {
		return imps.Entity(path), nil
	}
}

// defaultChannelName derives the channel name shown in harness logs and
// metrics.
func defaultChannelName(path string) string {
	return "soulstream:" + path
}
