// Package sqlite implements persistence using SQLite via sqlc-generated queries.
// It satisfies the consumer-defined interfaces in internal/auth/, internal/tasks/,
// and internal/domain/. The single Store struct composes all domain method files.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // register the sqlite3 driver with database/sql

	"github.com/pressly/goose/v3"

	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store is the SQLite-backed implementation of all persistence interfaces.
// When db is nil, the Store is scoped to a transaction (backed by a *sql.Tx).
type Store struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

func init() {
	// Both SetDialect and SetBaseFS write goose global state; call once at init to avoid races.
	if err := goose.SetDialect("sqlite3"); err != nil {
		panic("sqlite: goose.SetDialect: " + err.Error())
	}
	goose.SetBaseFS(migrationsFS)
}

// Open creates a Store by opening the SQLite file at path, applying pragmas,
// running embedded goose migrations, and seeding default settings.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := applyPragmas(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := goose.Up(db, "migrations"); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db, q: sqlcgen.New(db)}
	if err := s.ensureDefaultSettings(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// PingContext verifies the database connection is alive. Satisfies server.Pinger.
func (s *Store) PingContext(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("sqlite: cannot ping a transaction-scoped store")
	}
	return s.db.PingContext(ctx)
}

// Close shuts down the database connection. It must not be called on a tx-scoped Store.
func (s *Store) Close() error {
	if s.db == nil {
		return fmt.Errorf("sqlite: cannot close a transaction-scoped store")
	}
	return s.db.Close()
}

// WithTx executes fn within a database transaction. If fn returns an error the
// transaction is rolled back; otherwise it is committed. Calling WithTx on an
// already-tx-scoped Store is a passthrough that prevents accidental nesting.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	if s.db == nil {
		return fn(s)
	}
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txStore := &Store{db: nil, q: sqlcgen.New(sqlTx)}
	if err := fn(txStore); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	return sqlTx.Commit()
}

func ensureDir(path string) error {
	d := filepath.Dir(path)
	if d == "." {
		return nil
	}
	return os.MkdirAll(d, 0o755)
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

func tsFormat(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}
