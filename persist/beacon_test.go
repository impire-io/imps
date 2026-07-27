package persist

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBeacon_FirstStartIsAbsenceNotZero(t *testing.T) {
	ctx := context.Background()
	bc := NewBeacon("watcher", newMemBackend())
	elapsed, ok, err := bc.SleptFor(ctx)
	if err != nil {
		t.Fatalf("slept-for: %v", err)
	}
	if ok || elapsed != 0 {
		t.Errorf("first start = (%v, %v), want (0, false)", elapsed, ok)
	}
}

func TestBeacon_StampThenSleptFor(t *testing.T) {
	ctx := context.Background()
	clock := newFakeClock()
	bc := NewBeacon("watcher", newMemBackend())
	bc.now = clock.now

	if err := bc.Stamp(ctx); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	clock.advance(7 * time.Minute)
	elapsed, ok, err := bc.SleptFor(ctx)
	if err != nil {
		t.Fatalf("slept-for: %v", err)
	}
	if !ok || elapsed != 7*time.Minute {
		t.Errorf("slept-for = (%v, %v), want (7m, true)", elapsed, ok)
	}
}

func TestBeacon_BackendFailureSurfaces(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("backend down")
	bc := NewBeacon("watcher", failBackend{err: boom})
	if _, _, err := bc.SleptFor(ctx); !errors.Is(err, boom) {
		t.Errorf("SleptFor err = %v, want the backend error", err)
	}
	if err := bc.Stamp(ctx); !errors.Is(err, boom) {
		t.Errorf("Stamp err = %v, want the backend error", err)
	}
}
