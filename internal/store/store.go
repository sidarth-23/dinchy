// Package store defines shared persistence contracts.
// Each database backend lives in its own sub-package (sqlite/, postgres/, etc.)
// with its own sqlc queries, migrations, and generated code. All backends
// implement the same consumer-defined interfaces in domain/, auth/, and tasks/.
package store

import (
	"context"
	"database/sql"
)

// DBTX is the common interface satisfied by both *sql.DB and *sql.Tx,
// allowing store methods to execute within or outside a transaction.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
