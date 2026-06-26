// Package testutil provides shared test helpers for the Dinchy test suite.
// It is not a _test.go file so it can be imported by any test package.
package testutil

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sidarth-23/dinchy/internal/store/sqlite"
)

// OpenTestDB opens a fresh, fully-migrated SQLite database in a temporary directory.
// The database is automatically closed when the test finishes.
func OpenTestDB(t testing.TB) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("testutil.OpenTestDB: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
