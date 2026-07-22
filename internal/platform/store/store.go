// Package store is the postgres-backed persistence implementation.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

// Queries is the backend-neutral query contract used by the store package.
type Queries interface {
	sqlcgen.Querier
}

// Store owns a database connection or transaction and executes queries through the sqlc adapter.
type Store struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
	q    Queries
	newQ func(sqlcgen.DBTX) Queries
	name string
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

	s := &Store{pool: pool, q: newQueries(pool), newQ: newQueries, name: "postgres"}
	if err := s.EnsureDefaultSettings(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

// New opens a root store backed by pool.
func New(pool *pgxpool.Pool, name string, newQ func(sqlcgen.DBTX) Queries) *Store {
	return &Store{pool: pool, q: newQ(pool), newQ: newQ, name: name}
}

func newQueries(db sqlcgen.DBTX) Queries {
	return sqlcgen.New(db)
}

func newTxStore(tx pgx.Tx, name string, newQ func(sqlcgen.DBTX) Queries) *Store {
	return &Store{tx: tx, q: newQ(tx), newQ: newQ, name: name}
}

// Query returns the active backend query implementation.
func (s *Store) Query() Queries {
	return s.q
}

// Pool exposes the underlying connection pool for callers that need raw sqlc queries.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// PingContext verifies the database connection is alive.
func (s *Store) PingContext(ctx context.Context) error {
	if s.pool == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStorePing), apperrors.WithCause(fmt.Errorf("%s cannot ping a transaction-scoped store", s.name)))
	}
	return s.pool.Ping(ctx)
}

// Close shuts down the database connection.
func (s *Store) Close() error {
	if s.pool == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStoreClose), apperrors.WithCause(fmt.Errorf("%s cannot close a transaction-scoped store", s.name)))
	}
	s.pool.Close()
	return nil
}

// WithTx executes fn in a transaction.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	if s.tx != nil {
		if err := fn(s); err != nil {
			return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStoreTxPassthrough), apperrors.WithCause(err))
		}
		return nil
	}

	pgxTx, err := s.pool.Begin(ctx)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStoreTxBegin), apperrors.WithCause(err))
	}

	txStore := newTxStore(pgxTx, s.name, s.newQ)
	if err := fn(txStore); err != nil {
		if rbErr := pgxTx.Rollback(ctx); rbErr != nil {
			return errors.Join(
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStoreTxBody), apperrors.WithCause(err)),
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStoreTxRollback), apperrors.WithCause(rbErr)),
			)
		}
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStoreTxBody), apperrors.WithCause(err))
	}

	if err := pgxTx.Commit(ctx); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStoreTxCommit), apperrors.WithCause(err))
	}
	return nil
}
