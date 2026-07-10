package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// cacheKeySegment scopes every cache key under a segment distinct from other
// Redis users (event streams, SSO state) sharing the same key prefix.
const cacheKeySegment = "cache"

type redisCache struct {
	client *goredis.Client
	keyer  Keyer
}

// NewRedis returns a Redis-backed Cache, or a Noop when client is nil or enabled
// is false, so callers can wire a cache unconditionally.
func NewRedis(client *goredis.Client, keyer Keyer, enabled bool) Cache {
	if client == nil || !enabled {
		return Noop{}
	}
	return &redisCache{client: client, keyer: keyer}
}

func (c *redisCache) Get(ctx context.Context, key string) (value []byte, hit bool, err error) {
	value, err = c.client.Get(ctx, c.keyer.Key(cacheKeySegment, key)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get cache key %q: %w", key, err)
	}
	return value, true, nil
}

func (c *redisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.client.Set(ctx, c.keyer.Key(cacheKeySegment, key), value, ttl).Err(); err != nil {
		return fmt.Errorf("set cache key %q: %w", key, err)
	}
	return nil
}

func (c *redisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	namespaced := make([]string, len(keys))
	for i, key := range keys {
		namespaced[i] = c.keyer.Key(cacheKeySegment, key)
	}
	if err := c.client.Del(ctx, namespaced...).Err(); err != nil {
		return fmt.Errorf("delete %d cache keys: %w", len(keys), err)
	}
	return nil
}

func (c *redisCache) Enabled() bool { return true }
