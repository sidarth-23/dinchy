// Package store is the postgres-backed persistence implementation.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// Store owns the PostgreSQL connection pool.
type Store struct {
	pool *pgxpool.Pool
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

type openOptions struct {
	logger *slog.Logger
}

// Option configures Store creation.
type Option func(*openOptions)

// WithLogger enables pgx debug query tracing through logger.
func WithLogger(logger *slog.Logger) Option {
	return func(options *openOptions) {
		options.logger = logger
	}
}

// Open creates a PostgreSQL store, runs migrations, and seeds default settings.
func Open(ctx context.Context, dsn string, opts ...Option) (*Store, error) {
	var options openOptions
	for _, opt := range opts {
		opt(&options)
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(fmt.Errorf("parse postgres pool config: %w", err)))
	}
	if options.logger != nil {
		poolConfig.ConnConfig.Tracer = queryTracer(options)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		return nil, err
	}
	goose.SetBaseFS(migrationsFS)

	migrationConfig := *poolConfig.ConnConfig
	migrationDB := stdlib.OpenDB(migrationConfig)
	if err := goose.Up(migrationDB, "migrations"); err != nil {
		if closeErr := migrationDB.Close(); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}
	if err := migrationDB.Close(); err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(fmt.Errorf("close postgres migration handle: %w", err)))
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(fmt.Errorf("open postgres pool: %w", err)))
	}

	s := &Store{pool: pool}
	if err := s.EnsureDefaultSettings(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

// Pool exposes the underlying connection pool for callers that need raw sqlc queries.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// PingContext verifies the database connection is alive.
func (s *Store) PingContext(ctx context.Context) error {
	if s.pool == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStorePing), apperrors.WithCause(fmt.Errorf("cannot ping a store with no connection pool")))
	}
	return s.pool.Ping(ctx)
}

// Close shuts down the database connection.
func (s *Store) Close() error {
	if s.pool == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStoreClose), apperrors.WithCause(fmt.Errorf("cannot close a store with no connection pool")))
	}
	s.pool.Close()
	return nil
}
