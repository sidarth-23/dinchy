// Package cache selects the configured cache store backend.
package cache

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sidarth-23/dinchy/internal/cache/core"
	cacheredis "github.com/sidarth-23/dinchy/internal/cache/redis"
	"github.com/sidarth-23/dinchy/internal/config"
)

type Store interface {
	core.Store
	io.Closer
}

func Open(ctx context.Context, cfg config.CacheConfig) (Store, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "":
		return nil, nil
	case config.CacheBackendRedis:
		store, err := cacheredis.Open(cfg)
		if err != nil {
			return nil, err
		}
		if err := store.Ping(ctx); err != nil {
			_ = store.Close()
			return nil, err
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unsupported cache backend %q", cfg.Backend)
	}
}
