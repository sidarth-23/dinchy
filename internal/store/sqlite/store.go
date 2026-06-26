// Package sqlite implements persistence using SQLite via sqlc-generated queries.
// It satisfies the consumer-defined interfaces in internal/features/auth/,
// internal/features/tasks/, and internal/features/bootstrap/.
// The single Store struct composes all feature method files.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // register the sqlite3 driver with database/sql

	"github.com/pressly/goose/v3"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
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
		return nil, apperrors.Annotate(err,
			apperrors.WithMeta("operation", "Open"),
			apperrors.WithMeta("path", path),
		)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, apperrors.Internal(err, apperrors.WithMeta("operation", "sql.Open"), apperrors.WithMeta("path", path))
	}
	if err := applyPragmas(ctx, db); err != nil {
		return nil, closeWithErr(db, err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return nil, closeWithErr(db, err)
	}
	s := &Store{db: db, q: sqlcgen.New(db)}
	if err := s.ensureDefaultSettings(ctx); err != nil {
		return nil, closeWithErr(db, err)
	}
	return s, nil
}

// PingContext verifies the database connection is alive. Satisfies server.Pinger.
func (s *Store) PingContext(ctx context.Context) error {
	if s.db == nil {
		return apperrors.Internal(fmt.Errorf("sqlite cannot ping a transaction-scoped store"), apperrors.WithMeta("operation", "PingContext"))
	}
	return s.db.PingContext(ctx)
}

// Close shuts down the database connection. It must not be called on a tx-scoped Store.
func (s *Store) Close() error {
	if s.db == nil {
		return apperrors.Internal(fmt.Errorf("sqlite cannot close a transaction-scoped store"), apperrors.WithMeta("operation", "Close"))
	}
	return s.db.Close()
}

// WithTx executes fn within a database transaction. If fn returns an error the
// transaction is rolled back; otherwise it is committed. Calling WithTx on an
// already-tx-scoped Store is a passthrough that prevents accidental nesting.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	if s.db == nil {
		if err := fn(s); err != nil {
			return apperrors.Annotate(err,
				apperrors.WithMeta("operation", "WithTx"),
				apperrors.WithMeta("stage", "tx_passthrough"),
			)
		}
		return nil
	}
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithMeta("operation", "BeginTx"),
		)
	}
	txStore := &Store{db: nil, q: sqlcgen.New(sqlTx)}
	if err := fn(txStore); err != nil {
		if rbErr := sqlTx.Rollback(); rbErr != nil {
			return errors.Join(
				apperrors.Annotate(err, apperrors.WithMeta("operation", "WithTx"), apperrors.WithMeta("stage", "body")),
				apperrors.Annotate(rbErr, apperrors.WithMeta("operation", "Rollback")),
			)
		}
		return apperrors.Annotate(err,
			apperrors.WithMeta("operation", "WithTx"),
			apperrors.WithMeta("stage", "body"),
		)
	}
	if err := sqlTx.Commit(); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithMeta("operation", "Commit"),
		)
	}
	return nil
}

func ensureDir(path string) error {
	d := filepath.Dir(path)
	if d == "." {
		return nil
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return apperrors.Internal(err, apperrors.WithMeta("operation", "MkdirAll"), apperrors.WithMeta("path", d))
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
			return apperrors.Internal(err, apperrors.WithMeta("operation", "applyPragmas"), apperrors.WithMeta("pragma", p))
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

func closeWithErr(c interface{ Close() error }, cause error) error {
	if closeErr := c.Close(); closeErr != nil {
		return errors.Join(cause, closeErr)
	}
	return cause
}
