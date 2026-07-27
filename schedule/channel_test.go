package schedule

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	imps "github.com/impire-io/imps"
)

func TestChannel_Defaults(t *testing.T) {
	spec := Channel("SCHED", "ticks.reconcile")

	if spec.Name != "schedule:ticks.reconcile" {
		t.Errorf("Name = %q", spec.Name)
	}
	src, ok := spec.Source.(imps.StreamSource)
	if !ok {
		t.Fatalf("Source is %T, want imps.StreamSource", spec.Source)
	}
	if src.Stream != "SCHED" || src.FilterSubject != "ticks.reconcile" || src.Durable != "" {
		t.Errorf("source = %+v", src)
	}
	if src.ConsumerConfig.DeliverPolicy != jetstream.DeliverAllPolicy {
		t.Errorf("DeliverPolicy = %v, want DeliverAllPolicy", src.ConsumerConfig.DeliverPolicy)
	}
	entity, err := spec.ExtractEntity(Tick{})
	if err != nil || entity != "ticks.reconcile" {
		t.Errorf("default entity = (%q, %v), want target subject", entity, err)
	}
}

func TestChannel_DefaultDecodeIsHeaderOnly(t *testing.T) {
	spec := Channel("SCHED", "ticks.reconcile")

	h := nats.Header{}
	h.Set("Nats-Scheduler", "schedules.reconcile")
	h.Set("Nats-Schedule-Next", "2026-07-28T00:00:00Z")
	decoded, err := spec.Decode(imps.Message{
		Subject: "ticks.reconcile",
		Headers: h,
		Data:    []byte(`ignored — never parsed`),
	})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	tick := decoded.(Tick)
	if tick.Subject != "ticks.reconcile" || tick.Scheduler != "schedules.reconcile" || tick.Next != "2026-07-28T00:00:00Z" {
		t.Errorf("tick = %+v", tick)
	}

	// Missing headers → zero-valued fields, never an error.
	decoded, err = spec.Decode(imps.Message{Subject: "ticks.reconcile", Headers: nats.Header{}})
	if err != nil {
		t.Fatalf("decode without headers: %v", err)
	}
	if tick := decoded.(Tick); tick.Scheduler != "" || tick.Next != "" {
		t.Errorf("tick = %+v, want zero provenance", tick)
	}
}

func TestChannel_Options(t *testing.T) {
	start := time.Date(2026, 7, 28, 6, 0, 0, 0, time.UTC)

	spec := Channel("SCHED", "ticks.x",
		WithName("custom"),
		WithDurable("watcher"),
		WithStartSeq(9),
	)
	src := spec.Source.(imps.StreamSource)
	if spec.Name != "custom" || src.Durable != "watcher" {
		t.Errorf("name/durable not applied: %q %q", spec.Name, src.Durable)
	}
	if src.ConsumerConfig.DeliverPolicy != jetstream.DeliverByStartSequencePolicy || src.ConsumerConfig.OptStartSeq != 9 {
		t.Errorf("start-seq not applied: %+v", src.ConsumerConfig)
	}

	spec = Channel("SCHED", "ticks.x", WithStartTime(start))
	src = spec.Source.(imps.StreamSource)
	if src.ConsumerConfig.DeliverPolicy != jetstream.DeliverByStartTimePolicy ||
		src.ConsumerConfig.OptStartTime == nil || !src.ConsumerConfig.OptStartTime.Equal(start) {
		t.Errorf("start-time not applied: %+v", src.ConsumerConfig)
	}

	spec = Channel("SCHED", "ticks.x",
		WithDecoder(func(imps.Message) (any, error) { return "custom", nil }),
		WithEntityExtractor(func(any) (imps.Entity, error) { return "e", nil }),
	)
	if v, _ := spec.Decode(imps.Message{}); v != "custom" {
		t.Error("decoder override not applied")
	}
	if e, _ := spec.ExtractEntity(nil); e != "e" {
		t.Error("extractor override not applied")
	}
}
