package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/pressly/goose/v3"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store is the SQLite-backed persistence implementation.
type Store struct {
	*core.Store
}

// Open creates a SQLite store, runs migrations, and seeds default settings.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, err
	}
	goose.SetBaseFS(migrationsFS)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := applyPragmas(ctx, db); err != nil {
		return nil, closeWithErr(db, err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return nil, closeWithErr(db, err)
	}

	s := &Store{Store: core.New(db, "sqlite", func(dbtx core.DBTX) core.Queries {
		return newQueries(sqlcgen.New(dbtx))
	})}
	if err := s.EnsureDefaultSettings(ctx); err != nil {
		return nil, closeWithErr(db, err)
	}
	return s, nil
}

// WithTx wraps the shared transaction helper so callers keep the concrete sqlite.Store type.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	return s.Store.WithTx(ctx, func(tx *core.Store) error {
		return fn(&Store{Store: tx})
	})
}

func ensureDir(path string) error {
	d := filepath.Dir(path)
	if d == "." {
		return nil
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	return nil
}

func applyPragmas(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA temp_store=MEMORY;",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func closeWithErr(c interface{ Close() error }, cause error) error {
	if closeErr := c.Close(); closeErr != nil {
		return errors.Join(cause, closeErr)
	}
	return cause
}
