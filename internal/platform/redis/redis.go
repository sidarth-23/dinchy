// Package redis opens Redis connections and builds namespaced keys.
package redis

import (
	"context"
	"fmt"
	"io"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/sidarth-23/dinchy/internal/config"
)

// Client wraps a go-redis client.
type Client struct {
	client *goredis.Client
}

// Keyer builds colon-delimited Redis keys under a shared prefix.
type Keyer struct {
	prefix string
}

// Open connects to Redis using cfg and verifies the connection with a ping.
func Open(ctx context.Context, cfg config.RedisConfig) (*goredis.Client, error) {
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

// NewKeyer creates a Keyer that namespaces keys under prefix.
func NewKeyer(prefix string) Keyer {
	return Keyer{prefix: strings.Trim(prefix, ":")}
}

// Key joins the prefix and parts with colons, skipping empty segments.
func (k Keyer) Key(parts ...string) string {
	all := make([]string, 0, len(parts)+1)
	if k.prefix != "" {
		all = append(all, k.prefix)
	}
	for _, part := range parts {
		part = strings.Trim(part, ":")
		if part != "" {
			all = append(all, part)
		}
	}
	return strings.Join(all, ":")
}

var _ io.Closer = (*goredis.Client)(nil)
