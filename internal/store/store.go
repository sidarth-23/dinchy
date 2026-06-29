// Package store selects the concrete persistence backend.
package store

import (
	"context"
	"fmt"
	"io"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/tasks"
	"github.com/sidarth-23/dinchy/internal/store/postgres"
	"github.com/sidarth-23/dinchy/internal/store/sqlite"
)

// Store is the application-facing persistence contract.
type Store interface {
	auth.Store
	tasks.Store
	auth.SettingsReader
	io.Closer
	PingContext(ctx context.Context) error
}

// Open returns the configured backend implementation.
func Open(ctx context.Context, cfg config.Config) (Store, error) {
	switch cfg.DBBackend {
	case "", "sqlite":
		return sqlite.Open(ctx, cfg.DBPath)
	case "postgres":
		return postgres.Open(ctx, cfg.PostgresDSN)
	default:
		return nil, fmt.Errorf("unsupported database backend %q", cfg.DBBackend)
	}
}
