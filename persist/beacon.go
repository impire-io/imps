package persist

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Beacon is the imp-level sleep clock: an imp-scoped last-active stamp
// under its own backend key. Stamp liveness on a heartbeat and at shutdown;
// ask SleptFor at startup and run the imp-level wake step before imp.Run —
// a single call, before any channel dispatch resumes.
type Beacon struct {
	name string
	b    Backend
	now  func() time.Time // injectable from same-package tests
}

// NewBeacon builds the beacon. name is its backend key and must be
// non-empty (and distinct from any store's key space in shared buckets).
func NewBeacon(name string, b Backend) *Beacon {
	if name == "" {
		panic("persist: beacon name required")
	}
	return &Beacon{name: name, b: b, now: time.Now}
}

// Stamp records liveness now.
func (bc *Beacon) Stamp(ctx context.Context) error {
	return bc.b.Put(ctx, bc.name, []byte(bc.now().UTC().Format(time.RFC3339Nano)))
}

// SleptFor reads the elapsed interval since the last stamp. ok=false means
// the beacon was never stamped — a first-ever start, distinguishable from a
// zero-length sleep.
func (bc *Beacon) SleptFor(ctx context.Context) (elapsed time.Duration, ok bool, err error) {
	raw, err := bc.b.Get(ctx, bc.name)
	if errors.Is(err, ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	stamp, err := time.Parse(time.RFC3339Nano, string(raw))
	if err != nil {
		return 0, false, fmt.Errorf("persist: decode beacon stamp: %w", err)
	}
	return bc.now().Sub(stamp), true, nil
}
