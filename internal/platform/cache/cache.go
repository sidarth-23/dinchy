// Package cache selects the configured cache store backend.
package cache

import (
	"context"
	"io"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/cache/core"
	cacheredis "github.com/sidarth-23/dinchy/internal/platform/cache/redis"
)

type Store interface {
	core.Store
	io.Closer
}

// Open connects to the Redis cache. The cache is mandatory, so a healthy
// connection is required for startup to proceed.
func Open(ctx context.Context, cfg config.CacheConfig) (Store, error) {
	store, err := cacheredis.Open(cfg)
	if err != nil {
		return nil, err
	}
	if err := store.Ping(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}
