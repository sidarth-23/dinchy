package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	goredis "github.com/redis/go-redis/v9"

	"github.com/sidarth-23/dinchy/internal/config"
)

// OpenRedis connects to Redis using cfg and verifies the connection with a ping.
func OpenRedis(ctx context.Context, cfg config.RedisConfig) (*goredis.Client, error) {
	options := &goredis.Options{
		Addr:                  cfg.Addr,
		Username:              cfg.Username,
		Password:              cfg.Password,
		DB:                    cfg.Database,
		PoolSize:              cfg.PoolSize,
		MinIdleConns:          cfg.MinIdleConns,
		DialTimeout:           cfg.DialTimeout,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		ContextTimeoutEnabled: true,
	}
	if cfg.TLS {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: hostFromAddr(cfg.Addr)}
	}
	client := goredis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis at %q: %w", cfg.Addr, err)
	}
	return client, nil
}

func hostFromAddr(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
