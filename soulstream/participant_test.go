package soulstream

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/imps/testutil/natstest"
)

func TestNewParticipant_ErrorsLeaveConnOpen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	t.Run("jetstream unavailable", func(t *testing.T) {
		s := natstest.New(t) // JetStream deliberately NOT enabled
		nc, err := nats.Connect(s.URL())
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer nc.Close()

		if _, err := NewParticipant(ctx, nc, "testrealm", "imp"); err == nil {
			t.Fatal("expected error against a server without JetStream")
		}
		if nc.IsClosed() {
			t.Fatal("NewParticipant closed the imp's connection on failure")
		}
	})

	t.Run("invalid slugs", func(t *testing.T) {
		s := natstest.New(t)
		s.JetStream(t)
		nc, err := nats.Connect(s.URL())
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer nc.Close()

		if _, err := NewParticipant(ctx, nc, "Bad Realm", "imp"); err == nil || !strings.Contains(err.Error(), "realm") {
			t.Errorf("bad realm slug: err = %v", err)
		}
		if _, err := NewParticipant(ctx, nc, "testrealm", "Bad Persona"); err == nil || !strings.Contains(err.Error(), "persona") {
			t.Errorf("bad persona slug: err = %v", err)
		}
		if nc.IsClosed() {
			t.Fatal("NewParticipant closed the imp's connection on validation failure")
		}
	})
}

func TestNewParticipant_ReadOnlyAndConnOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	s := natstest.New(t)
	s.JetStream(t)
	nc, err := nats.Connect(s.URL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	// Empty persona constructs a read-only participant.
	p, err := NewParticipant(ctx, nc, "testrealm", "")
	if err != nil {
		t.Fatalf("read-only participant: %v", err)
	}
	if p.Topic("whatever") == nil {
		t.Fatal("Topic returned nil handle")
	}

	// The connection stays open and usable after construction and use.
	if nc.IsClosed() {
		t.Fatal("participant closed the wrapped connection")
	}
	if err := nc.Publish("ownership.probe", []byte("still mine")); err != nil {
		t.Fatalf("connection unusable after participant construction: %v", err)
	}
}
