package cache

import (
	"context"
	"time"
)

// Noop is a Cache that stores nothing; every Get reports a miss. It is the
// backing used when caching is disabled or no Redis client is available.
type Noop struct{}

// Get always reports a miss.
func (Noop) Get(context.Context, string) (value []byte, hit bool, err error) {
	return nil, false, nil
}

// Set discards the value.
func (Noop) Set(context.Context, string, []byte, time.Duration) error { return nil }

// Delete does nothing.
func (Noop) Delete(context.Context, ...string) error { return nil }

// Enabled reports that the cache performs no work.
func (Noop) Enabled() bool { return false }
