// Package core defines backend-neutral cache store contracts.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Store interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

type Keyer struct {
	prefix string
}

func NewKeyer(prefix string) Keyer {
	prefix = strings.Trim(strings.TrimSpace(prefix), ":")
	return Keyer{prefix: prefix}
}

func (k Keyer) Key(parts ...string) string {
	clean := make([]string, 0, len(parts)+1)
	if k.prefix != "" {
		clean = append(clean, k.prefix)
	}
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), ":")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, ":")
}

func SetJSON(ctx context.Context, store Store, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal cache value for key %q: %w", key, err)
	}
	if err := store.Set(ctx, key, raw, ttl); err != nil {
		return fmt.Errorf("set cache key %q: %w", key, err)
	}
	return nil
}

func GetJSON(ctx context.Context, store Store, key string, target any) error {
	raw, err := store.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("get cache key %q: %w", key, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("unmarshal cache value for key %q: %w", key, err)
	}
	return nil
}
