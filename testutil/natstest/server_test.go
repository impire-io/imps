package natstest_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/imps/testutil/natstest"
)

func TestServer_CoreOnly(t *testing.T) {
	srv := natstest.New(t)
	nc, err := nats.Connect(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	if err := nc.Publish("foo", []byte("bar")); err != nil {
		t.Fatal(err)
	}
}

func TestServer_JetStreamEnabled(t *testing.T) {
	srv := natstest.New(t)
	js := srv.JetStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "TEST",
		Subjects: []string{"test.>"},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
}
