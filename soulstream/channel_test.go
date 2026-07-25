package soulstream

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream/record"
	"github.com/impire-io/soulstream/topic"

	imps "github.com/impire-io/imps"
)

func TestTopicChannel_Defaults(t *testing.T) {
	const path = "vat-q2-abcd"
	spec := TopicChannel(path)

	if spec.Name != "soulstream:"+path {
		t.Errorf("Name = %q, want %q", spec.Name, "soulstream:"+path)
	}
	src, ok := spec.Source.(imps.StreamSource)
	if !ok {
		t.Fatalf("Source is %T, want imps.StreamSource", spec.Source)
	}
	if src.Stream != "SOULSTREAM" {
		t.Errorf("Stream = %q, want SOULSTREAM", src.Stream)
	}
	if want := topic.OpsSubject(path); src.FilterSubject != want {
		t.Errorf("FilterSubject = %q, want %q", src.FilterSubject, want)
	}
	if src.Durable != "" {
		t.Errorf("Durable = %q, want ephemeral default", src.Durable)
	}
	if src.ConsumerConfig.DeliverPolicy != jetstream.DeliverAllPolicy {
		t.Errorf("DeliverPolicy = %v, want DeliverAllPolicy", src.ConsumerConfig.DeliverPolicy)
	}
	if spec.Decode == nil || spec.ExtractEntity == nil {
		t.Fatal("Decode and ExtractEntity must have defaults")
	}

	// Default extractor: the topic path is the entity, whatever the op.
	entity, err := spec.ExtractEntity(Op{Type: "turn.post"})
	if err != nil || entity != imps.Entity(path) {
		t.Errorf("default entity = (%q, %v), want (%q, nil)", entity, err, path)
	}
}

func TestTopicChannel_DefaultDecodeIsHeaderOnly(t *testing.T) {
	spec := TopicChannel("some-topic")

	h := nats.Header{}
	h.Set(record.HeaderType, "turn.post")
	h.Set(record.HeaderAuthor, "scribe")
	h.Set(record.HeaderMsgID, "op-123")
	decoded, err := spec.Decode(imps.Message{
		Subject: topic.OpsSubject("some-topic"),
		Headers: h,
		Data:    []byte(`{"deliberately":"not parsed`), // invalid JSON must not matter
	})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	op, ok := decoded.(Op)
	if !ok {
		t.Fatalf("decoded is %T, want Op", decoded)
	}
	if op.Type != "turn.post" || op.Author != "scribe" || op.ID != "op-123" {
		t.Errorf("op = %+v", op)
	}

	// Missing headers decode to zero values, never an error: malformed ops
	// are delivered and awareness judges.
	decoded, err = spec.Decode(imps.Message{Headers: nats.Header{}})
	if err != nil {
		t.Fatalf("decode with no headers: %v", err)
	}
	if op := decoded.(Op); op != (Op{}) {
		t.Errorf("op = %+v, want zero Op", op)
	}
}

func TestTopicChannel_Options(t *testing.T) {
	const path = "opts-topic"
	start := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	spec := TopicChannel(path,
		WithName("custom"),
		WithDurable("watcher-1"),
		WithStartSeq(42),
	)
	src := spec.Source.(imps.StreamSource)
	if spec.Name != "custom" {
		t.Errorf("Name = %q, want custom", spec.Name)
	}
	if src.Durable != "watcher-1" {
		t.Errorf("Durable = %q, want watcher-1", src.Durable)
	}
	if src.ConsumerConfig.DeliverPolicy != jetstream.DeliverByStartSequencePolicy || src.ConsumerConfig.OptStartSeq != 42 {
		t.Errorf("start-seq option not applied: %+v", src.ConsumerConfig)
	}

	spec = TopicChannel(path, WithStartTime(start))
	src = spec.Source.(imps.StreamSource)
	if src.ConsumerConfig.DeliverPolicy != jetstream.DeliverByStartTimePolicy ||
		src.ConsumerConfig.OptStartTime == nil || !src.ConsumerConfig.OptStartTime.Equal(start) {
		t.Errorf("start-time option not applied: %+v", src.ConsumerConfig)
	}

	customDecode := func(imps.Message) (any, error) { return "custom", nil }
	customExtract := func(any) (imps.Entity, error) { return "custom-entity", nil }
	spec = TopicChannel(path, WithDecoder(customDecode), WithEntityExtractor(customExtract))
	if v, _ := spec.Decode(imps.Message{}); v != "custom" {
		t.Errorf("decoder override not applied")
	}
	if e, _ := spec.ExtractEntity(nil); e != "custom-entity" {
		t.Errorf("extractor override not applied")
	}
}
