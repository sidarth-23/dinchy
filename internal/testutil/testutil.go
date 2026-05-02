// Package testutil provides shared test helpers for the Dinchy test suite.
// It is not a _test.go file so it can be imported by any test package.
package testutil

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

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

// NewController creates a gomock controller that calls Finish automatically via t.Cleanup.
func NewController(t testing.TB) *gomock.Controller {
	t.Helper()
	return gomock.NewController(t)
}

// FakeClock is a controllable clock for use in tests.
type FakeClock struct {
	mu sync.Mutex
	t  time.Time
}

// NewFakeClock returns a FakeClock set to the given time.
func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{t: t}
}

// Now returns the current fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// Set replaces the current fake time.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}
