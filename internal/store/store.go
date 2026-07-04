// Package store selects the concrete persistence backend.
package store

import (
	"context"
	"fmt"
	"io"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/features/audit"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/store/postgres"
	"github.com/sidarth-23/dinchy/internal/store/sqlite"
	"github.com/sidarth-23/dinchy/internal/workers"
)

// Store is the application-facing persistence contract.
type Store interface {
	auth.Store
	audit.Store
	workers.Store
	auth.SettingsReader
	io.Closer
	PingContext(ctx context.Context) error
}

// Open returns the configured backend implementation.
func Open(ctx context.Context, cfg config.Config) (Store, error) {
	switch cfg.Database.DBBackend {
	case "", config.DBBackendSQLite:
		return sqlite.Open(ctx, cfg.Database.DBPath)
	case config.DBBackendPostgres:
		return postgres.Open(ctx, cfg.Database.PostgresDSN)
	default:
		return nil, fmt.Errorf("unsupported database backend %q", cfg.Database.DBBackend)
	}
}
