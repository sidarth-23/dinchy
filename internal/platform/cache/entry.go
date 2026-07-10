package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Entry is a typed, namespaced handle over a Cache. It owns the key format, the
// value type, and the default TTL, so consumers address values by identifier
// alone and never construct keys or repeat TTLs.
type Entry[T any] struct {
	cache      Cache
	namespace  string
	defaultTTL time.Duration
}

// NewEntry builds an Entry that stores values of type T under namespace, using
// defaultTTL for Set. A nil or disabled cache turns every operation into a no-op.
func NewEntry[T any](c Cache, namespace string, defaultTTL time.Duration) Entry[T] {
	return Entry[T]{cache: c, namespace: namespace, defaultTTL: defaultTTL}
}

func (e Entry[T]) key(id string) string {
	return e.namespace + ":" + id
}

func (e Entry[T]) disabled() bool {
	return e.cache == nil || !e.cache.Enabled()
}

// Enabled reports whether the entry performs real work. Callers can use it to
// skip work (such as extra queries) that only feeds the cache.
func (e Entry[T]) Enabled() bool {
	return !e.disabled()
}

// Get returns the value stored for id and whether it was present. A nil or
// disabled cache always misses.
func (e Entry[T]) Get(ctx context.Context, id string) (value T, hit bool, err error) {
	if e.disabled() {
		return value, false, nil
	}
	data, hit, err := e.cache.Get(ctx, e.key(id))
	if err != nil {
		return value, false, err
	}
	if !hit {
		return value, false, nil
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
	if e.disabled() {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cached value for %q: %w", e.key(id), err)
	}
	return e.cache.Set(ctx, e.key(id), data, ttl)
}

// Delete removes the cached values for the given ids.
func (e Entry[T]) Delete(ctx context.Context, ids ...string) error {
	if e.disabled() || len(ids) == 0 {
		return nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = e.key(id)
	}
	return e.cache.Delete(ctx, keys...)
}
