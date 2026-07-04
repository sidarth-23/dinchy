// Package store is the postgres-backed persistence implementation.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/postgres/sqlcgen"
)

//go:embed postgres/migrations/*.sql
var migrationsFS embed.FS

// Store is the PostgreSQL-backed persistence implementation.
type Store struct {
	*core.Store
}

// Open creates a PostgreSQL store, runs migrations, and seeds default settings.
func Open(ctx context.Context, dsn string) (*Store, error) {
	if err := goose.SetDialect("postgres"); err != nil {
		return nil, err
	}
	goose.SetBaseFS(migrationsFS)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := goose.Up(db, "postgres/migrations"); err != nil {
		return nil, closeWithErr(db, err)
	}

	s := &Store{Store: core.New(db, "postgres", func(dbtx core.DBTX) core.Queries {
		return newQueries(sqlcgen.New(dbtx))
	})}
	if err := s.EnsureDefaultSettings(ctx); err != nil {
		return nil, closeWithErr(db, err)
	}
	return s, nil
}

// WithTx wraps the shared transaction helper so callers keep the concrete Store type.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	return s.Store.WithTx(ctx, func(tx *core.Store) error {
		return fn(&Store{Store: tx})
	})
}

func closeWithErr(c interface{ Close() error }, cause error) error {
	if closeErr := c.Close(); closeErr != nil {
		return errors.Join(cause, closeErr)
	}
	return cause
}
