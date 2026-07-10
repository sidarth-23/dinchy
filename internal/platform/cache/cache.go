// Package cache provides an optional, Redis-backed read-through cache exposed
// through typed, namespaced entries so callers never build key strings by hand.
package cache

import (
	"context"
	"time"
)

// Cache is the low-level byte-oriented cache surface. Consumers should prefer
// Entry, which layers key namespacing and typed values on top of this.
type Cache interface {
	// Get returns the stored bytes and whether the key was present.
	Get(ctx context.Context, key string) (value []byte, hit bool, err error)
	// Set stores value under key with the given time-to-live.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Delete removes the given keys, ignoring any that are absent.
	Delete(ctx context.Context, keys ...string) error
	// Enabled reports whether the cache performs real work.
	Enabled() bool
}
