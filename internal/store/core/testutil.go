package core

import (
	"context"
	"testing"
)

type testStore interface {
	PingContext(context.Context) error
	Close() error
}

// OpenTestDB opens a fully migrated test store and registers cleanup for it.
// The opener can be sqlite.Open, postgres.Open, or any compatible backend opener.
func OpenTestDB[T testStore](t testing.TB, open func(context.Context, string) (T, error), conn string) T {
	t.Helper()

	db, err := open(context.Background(), conn)
	if err != nil {
		t.Fatalf("core.OpenTestDB: %v", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("core.OpenTestDB ping: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })
	return db
}
