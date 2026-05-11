// Package natstest spins up an embedded nats-server per call for tests.
// JetStream is enabled on demand via JetStream(t). Cleanup is wired through
// t.Cleanup so callers get teardown for free.
package natstest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Server wraps a running nats-server and the URL clients connect to.
type Server struct {
	t   *testing.T
	srv *natsserver.Server
	url string
}

// New starts a fresh embedded nats-server on a random port. JetStream is
// disabled until JetStream(t) is called. The caller does not need to stop
// the server explicitly — t.Cleanup handles it.
func New(t *testing.T) *Server {
	t.Helper()
	opts := &natsserver.Options{
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 true,
		NoSigs:                true,
		DisableShortFirstPing: true,
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("natstest: NewServer: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatalf("natstest: server not ready in 5s")
	}
	s := &Server{t: t, srv: srv, url: srv.ClientURL()}
	t.Cleanup(s.Shutdown)
	return s
}

// URL returns the client URL of the embedded server.
func (s *Server) URL() string {
	return s.url
}

// Shutdown stops the server. Safe to call more than once. Wired into
// t.Cleanup by New, so callers rarely need to invoke it.
func (s *Server) Shutdown() {
	if s.srv != nil {
		s.srv.Shutdown()
		s.srv.WaitForShutdown()
	}
}

// JetStream enables JetStream on the server with a temporary store
// directory. Returns a JetStream context bound to a fresh connection.
// Subsequent calls reuse the existing JetStream config.
func (s *Server) JetStream(t *testing.T) jetstream.JetStream {
	t.Helper()
	if !s.srv.JetStreamEnabled() {
		dir, err := os.MkdirTemp("", "natstest-js-*")
		if err != nil {
			t.Fatalf("natstest: MkdirTemp: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })

		if err := s.srv.EnableJetStream(&natsserver.JetStreamConfig{
			StoreDir: filepath.Join(dir, "store"),
		}); err != nil {
			t.Fatalf("natstest: EnableJetStream: %v", err)
		}
		if !s.srv.ReadyForConnections(5 * time.Second) {
			t.Fatalf("natstest: server not ready after EnableJetStream")
		}
	}
	nc, err := nats.Connect(s.url)
	if err != nil {
		t.Fatalf("natstest: nats.Connect: %v", err)
	}
	t.Cleanup(func() { nc.Close() })
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("natstest: jetstream.New: %v", err)
	}
	return js
}
