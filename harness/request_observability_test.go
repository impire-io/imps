package harness

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// TestRequestLogLines asserts the dispatch helpers emit the log messages
// named in contracts/request-reply.md § Observability: a DEBUG "request"
// on success, a WARN "request failed" on failure, and a DEBUG
// "request_many" on completion.
func TestRequestLogLines(t *testing.T) {
	url := runEmbeddedServer(t)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })

	if _, err := nc.Subscribe("ok.subject", func(m *nats.Msg) {
		_ = m.Respond([]byte("pong"))
	}); err != nil {
		t.Fatal(err)
	}
	// Unrelated subscriber so the connection isn't idle; ensures the
	// substrate returns ErrNoResponders for "missing.subject" rather than
	// behaving oddly under a fully empty subjects table.
	if _, err := nc.Subscribe("unrelated", func(_ *nats.Msg) {}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	lg := newLogger(handler)
	m := newMetrics()

	if _, err := requestSingle(
		context.Background(), nc, m, lg,
		time.Second, "ok.subject", []byte("ping"), nil,
	); err != nil {
		t.Fatalf("requestSingle ok: %v", err)
	}

	// Failure path: no-responders. Subject has no subscriber, so the
	// substrate short-circuits and the helper emits a "request failed"
	// WARN with category=no_responders.
	_, err = requestSingle(
		context.Background(), nc, m, lg,
		time.Second, "missing.subject", nil, nil,
	)
	var noResp *ErrNoResponders
	if !errors.As(err, &noResp) {
		t.Fatalf("err = %T %v, want *ErrNoResponders", err, err)
	}

	if _, err := requestMany(
		context.Background(), nc, m, lg,
		100*time.Millisecond, "ok.subject", nil, nil,
	); err != nil {
		t.Fatalf("requestMany: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		`msg=request `, // DEBUG "request" — success path
		`msg="request failed"`,
		`msg=request_many `, // DEBUG "request_many" — completion path
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected log output to contain %q; got:\n%s", want, output)
		}
	}
}
