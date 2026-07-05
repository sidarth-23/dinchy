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
	"github.com/sidarth-23/dinchy/internal/platform/cache/core"
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

func (s *Store) CreateConsumerGroup(ctx context.Context, stream, group string) error {
	err := s.client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("redis create stream consumer group %q for stream %q: %w", group, stream, err)
	}
	return nil
}

func (s *Store) AddStream(ctx context.Context, stream string, values map[string]any, retention time.Duration) (string, error) {
	args := &goredis.XAddArgs{
		Stream: stream,
		Values: values,
	}
	if retention > 0 {
		cutoff := time.Now().UTC().Add(-retention)
		args.MinID = fmt.Sprintf("%d-0", cutoff.UnixMilli())
		args.Approx = true
	}
	id, err := s.client.XAdd(ctx, args).Result()
	if err != nil {
		return "", fmt.Errorf("redis add stream entry to %q: %w", stream, err)
	}
	return id, nil
}

func (s *Store) ReadGroup(ctx context.Context, stream, group, consumer string, count int64, block, claim time.Duration) ([]core.StreamMessage, error) {
	streams, err := s.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    block,
		Claim:    claim,
	}).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("redis read stream %q group %q: %w", stream, group, err)
	}
	out := []core.StreamMessage{}
	for _, redisStream := range streams {
		for _, message := range redisStream.Messages {
			values := make(map[string]string, len(message.Values))
			for key, value := range message.Values {
				values[key] = fmt.Sprint(value)
			}
			out = append(out, core.StreamMessage{ID: message.ID, Values: values})
		}
	}
	return out, nil
}

func (s *Store) AckStream(ctx context.Context, stream, group string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.client.XAck(ctx, stream, group, ids...).Err(); err != nil {
		return fmt.Errorf("redis ack stream %q group %q: %w", stream, group, err)
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
var _ core.StreamStore = (*Store)(nil)
