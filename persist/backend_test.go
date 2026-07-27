package persist

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/imps/testutil/natstest"
)

// memBackend is the in-memory Backend for deterministic unit tests. It
// counts operations so eviction purity (no writes, no deletes) is
// assertable.
type memBackend struct {
	mu      sync.Mutex
	m       map[string][]byte
	gets    int
	puts    int
	deletes int
}

func newMemBackend() *memBackend {
	return &memBackend{m: make(map[string][]byte)}
}

func (b *memBackend) Get(_ context.Context, key string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gets++
	v, ok := b.m[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), v...), nil
}

func (b *memBackend) Put(_ context.Context, key string, value []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.puts++
	b.m[key] = append([]byte(nil), value...)
	return nil
}

func (b *memBackend) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deletes++
	delete(b.m, key)
	return nil
}

func (b *memBackend) counts() (gets, puts, deletes int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gets, b.puts, b.deletes
}

// failBackend fails every operation with its configured error.
type failBackend struct{ err error }

func (b failBackend) Get(context.Context, string) ([]byte, error) { return nil, b.err }
func (b failBackend) Put(context.Context, string, []byte) error   { return b.err }
func (b failBackend) Delete(context.Context, string) error        { return b.err }

func TestKVBackend_ReferenceImplementation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := natstest.New(t)
	js := s.JetStream(t)
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "imp-state"})
	if err != nil {
		t.Fatalf("kv: %v", err)
	}
	b := KVBackend(kv)

	// Missing key → ErrNotFound (errors.Is-matchable).
	if _, err := b.Get(ctx, "customers.nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing: err = %v, want ErrNotFound", err)
	}
	// Round-trip.
	if err := b.Put(ctx, "customers.c1", []byte("v1")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := b.Get(ctx, "customers.c1")
	if err != nil || string(got) != "v1" {
		t.Errorf("Get = (%q, %v), want (v1, nil)", got, err)
	}
	// Delete existing → subsequent Get is ErrNotFound.
	if err := b.Delete(ctx, "customers.c1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := b.Get(ctx, "customers.c1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: err = %v, want ErrNotFound", err)
	}
	// Delete absent → not an error.
	if err := b.Delete(ctx, "customers.never-existed"); err != nil {
		t.Errorf("Delete absent: err = %v, want nil", err)
	}
}
