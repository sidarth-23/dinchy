package cache

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"github.com/sidarth-23/dinchy/internal/config"
)

// OpenRedis connects to Redis using cfg and verifies the connection with a ping.
func OpenRedis(ctx context.Context, cfg config.RedisConfig) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.Database,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis at %q: %w", cfg.Addr, err)
	}
	return client, nil
}
