// Package redis implements the generic cache store contract using Redis.
package redis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/sidarth-23/dinchy/internal/config"
)

type Store struct {
	client *goredis.Client
}

func Open(cfg config.CacheConfig) (*Store, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, fmt.Errorf("DINCHY_CACHE_ADDR is required for cache backend %q", cfg.Backend)
	}
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.Database,
	})
	return &Store{client: client}, nil
}

func (s *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := s.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set key %q: %w", key, err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	raw, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, fmt.Errorf("redis key %q not found: %w", key, err)
		}
		return nil, fmt.Errorf("redis get key %q: %w", key, err)
	}
	return raw, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis delete key %q: %w", key, err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

var _ io.Closer = (*Store)(nil)
