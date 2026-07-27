package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/imps/testutil/natstest"
)

func newScheduleStream(ctx context.Context, t *testing.T, js jetstream.JetStream) jetstream.Stream {
	t.Helper()
	st, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:              "SCHED",
		Subjects:          []string{"schedules.>", "ticks.>", "state.>"},
		AllowMsgSchedules: true,
		AllowMsgTTL:       true,
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	return st
}

func storedHeaders(ctx context.Context, t *testing.T, st jetstream.Stream, subj string) nats.Header {
	t.Helper()
	raw, err := st.GetLastMsgForSubject(ctx, subj)
	if err != nil {
		t.Fatalf("read back %s: %v", subj, err)
	}
	return raw.Header
}

func TestRegister_HeaderRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := natstest.New(t)
	js := s.JetStream(t)
	st := newScheduleStream(ctx, t, js)

	// All options → every implied header, correctly formatted.
	err := Register(ctx, js, "schedules.full", "0 0 12 * * *", "ticks.full",
		WithTickTTL(90*time.Second),
		WithTimeZone("Europe/Brussels"),
		WithBody([]byte("payload")),
		WithSource("state.latest"),
		WithRollup(),
	)
	if err != nil {
		t.Fatalf("register full: %v", err)
	}
	h := storedHeaders(ctx, t, st, "schedules.full")
	for header, want := range map[string]string{
		"Nats-Schedule":           "0 0 12 * * *",
		"Nats-Schedule-Target":    "ticks.full",
		"Nats-Schedule-TTL":       "1m30s",
		"Nats-Schedule-Time-Zone": "Europe/Brussels",
		"Nats-Schedule-Source":    "state.latest",
		"Nats-Schedule-Rollup":    "sub",
	} {
		if got := h.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	// Minimal register → only the two required headers.
	if err := Register(ctx, js, "schedules.min", "@every 1h", "ticks.min"); err != nil {
		t.Fatalf("register minimal: %v", err)
	}
	h = storedHeaders(ctx, t, st, "schedules.min")
	if h.Get("Nats-Schedule") != "@every 1h" || h.Get("Nats-Schedule-Target") != "ticks.min" {
		t.Errorf("required headers wrong: %v", h)
	}
	for _, header := range []string{"Nats-Schedule-TTL", "Nats-Schedule-Time-Zone", "Nats-Schedule-Source", "Nats-Schedule-Rollup"} {
		if got := h.Get(header); got != "" {
			t.Errorf("minimal register wrote %s = %q", header, got)
		}
	}

	// Re-register the same subject → replaced, not duplicated.
	if err := Register(ctx, js, "schedules.min", "@every 2h", "ticks.min"); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if got := storedHeaders(ctx, t, st, "schedules.min").Get("Nats-Schedule"); got != "@every 2h" {
		t.Errorf("replacement not stored: pattern = %q", got)
	}

	// Deregister → schedule gone.
	if err := Deregister(ctx, js, "SCHED", "schedules.min"); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if _, err := st.GetLastMsgForSubject(ctx, "schedules.min"); !errors.Is(err, jetstream.ErrMsgNotFound) {
		t.Errorf("schedule still present after deregister: %v", err)
	}
}

func TestRegister_FailFastValidation(t *testing.T) {
	ctx := context.Background()
	// nil JetStream handle: any substrate contact would panic — passing
	// validation-failure cases through proves zero substrate contact.
	cases := []struct {
		name string
		call func() error
	}{
		{"empty pattern", func() error { return Register(ctx, nil, "schedules.x", "", "ticks.x") }},
		{"empty target", func() error { return Register(ctx, nil, "schedules.x", "@every 1s", "") }},
		{"empty schedule subject", func() error { return Register(ctx, nil, "", "@every 1s", "ticks.x") }},
		{"zero TTL", func() error { return Register(ctx, nil, "schedules.x", "@every 1s", "ticks.x", WithTickTTL(0)) }},
		{"negative TTL", func() error {
			return Register(ctx, nil, "schedules.x", "@every 1s", "ticks.x", WithTickTTL(-time.Second))
		}},
		{"deregister empty", func() error { return Deregister(ctx, nil, "", "schedules.x") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("expected a fail-fast error")
			}
		})
	}
}

func TestRegister_ReplacementTakesEffectOnFiring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s := natstest.New(t)
	js := s.JetStream(t)
	st := newScheduleStream(ctx, t, js)

	// A slow schedule first: no tick in a short observation window.
	if err := Register(ctx, js, "schedules.switch", "@every 1h", "ticks.switch"); err != nil {
		t.Fatalf("register slow: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := st.GetLastMsgForSubject(ctx, "ticks.switch"); !errors.Is(err, jetstream.ErrMsgNotFound) {
		t.Fatalf("slow schedule fired unexpectedly (or read failed): %v", err)
	}

	// Replace with a fast pattern: a tick arrives within seconds — the
	// replacement, not the original, governs the next firing.
	if err := Register(ctx, js, "schedules.switch", "@every 1s", "ticks.switch"); err != nil {
		t.Fatalf("register fast: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := st.GetLastMsgForSubject(ctx, "ticks.switch"); err == nil {
			if got := raw.Header.Get("Nats-Scheduler"); got != "schedules.switch" {
				t.Errorf("tick provenance = %q", got)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no tick after replacing with a fast pattern")
}
