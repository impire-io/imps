package persist

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

// ErrNotFound reports a key absent from the backend.
var ErrNotFound = errors.New("persist: not found")

// Backend is the minimal persistence boundary. The framework commits to no
// implementation; KVBackend is the reference. Implementations must return
// ErrNotFound (errors.Is-matchable) from Get on a missing key, and must not
// treat Delete of an absent key as an error.
type Backend interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
}

// KVBackend adapts a JetStream key-value bucket (the reference backend).
// The bucket is provisioned by the operator; the adapter never creates it.
// Keys handed to it must be valid KV keys — the store's "<name>.<entity>"
// shape is valid whenever the name and entity use the KV key charset.
func KVBackend(kv jetstream.KeyValue) Backend {
	return &kvBackend{kv: kv}
}

type kvBackend struct {
	kv jetstream.KeyValue
}

func (b *kvBackend) Get(ctx context.Context, key string) ([]byte, error) {
	e, err := b.kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("persist: kv get %s: %w", key, err)
	}
	return e.Value(), nil
}

func (b *kvBackend) Put(ctx context.Context, key string, value []byte) error {
	if _, err := b.kv.Put(ctx, key, value); err != nil {
		return fmt.Errorf("persist: kv put %s: %w", key, err)
	}
	return nil
}

func (b *kvBackend) Delete(ctx context.Context, key string) error {
	if err := b.kv.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("persist: kv delete %s: %w", key, err)
	}
	return nil
}
