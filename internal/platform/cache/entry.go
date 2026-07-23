// Package cache provides Redis connection setup, shared key namespacing, and a
// typed, namespaced cache entry so callers address cached values by identifier
// alone and never build key strings by hand.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// cacheKeySegment scopes every cache key under a segment distinct from other
// Redis users (event streams, SSO state) sharing the same key prefix.
const cacheKeySegment = "cache"

// Entry is a typed, namespaced handle over Redis. It owns the key format, the
// value type, JSON serialization, and the default TTL. A nil client disables the
// entry, turning every operation into a no-op.
type Entry[T any] struct {
	client     *goredis.Client
	keyer      Keyer
	namespace  string
	defaultTTL time.Duration
}

// NewEntry builds an Entry that stores values of type T under namespace, using
// defaultTTL for Set. A nil client turns every operation into a no-op.
func NewEntry[T any](client *goredis.Client, keyer Keyer, namespace string, defaultTTL time.Duration) Entry[T] {
	return Entry[T]{client: client, keyer: keyer, namespace: namespace, defaultTTL: defaultTTL}
}

func (e Entry[T]) key(id string) string {
	return e.keyer.Key(cacheKeySegment, e.namespace, id)
}

// Enabled reports whether the entry performs real work. Callers can use it to
// skip work (such as extra queries) that only feeds the cache.
func (e Entry[T]) Enabled() bool {
	return e.client != nil
}

// Get returns the value stored for id and whether it was present. A disabled
// entry always misses.
func (e Entry[T]) Get(ctx context.Context, id string) (value T, hit bool, err error) {
	if e.client == nil {
		return value, false, nil
	}
	data, err := e.client.Get(ctx, e.key(id)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return value, false, nil
		}
		return value, false, fmt.Errorf("get cache key %q: %w", e.key(id), err)
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, false, fmt.Errorf("decode cached value for %q: %w", e.key(id), err)
	}
	return value, true, nil
}

// Set stores value for id using the entry's default TTL.
func (e Entry[T]) Set(ctx context.Context, id string, value T) error {
	return e.SetWithTTL(ctx, id, value, e.defaultTTL)
}

// SetWithTTL stores value for id with an explicit TTL, overriding the default.
func (e Entry[T]) SetWithTTL(ctx context.Context, id string, value T, ttl time.Duration) error {
	if e.client == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cached value for %q: %w", e.key(id), err)
	}
	if err := e.client.Set(ctx, e.key(id), data, ttl).Err(); err != nil {
		return fmt.Errorf("set cache key %q: %w", e.key(id), err)
	}
	return nil
}

// Delete removes the cached values for the given ids.
func (e Entry[T]) Delete(ctx context.Context, ids ...string) error {
	if e.client == nil || len(ids) == 0 {
		return nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = e.key(id)
	}
	if err := e.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("delete %d cache keys: %w", len(ids), err)
	}
	return nil
}
