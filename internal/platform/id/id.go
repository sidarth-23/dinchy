// Package id provides monotonic ULID generation.
package id

import (
	"sync"

	"github.com/oklog/ulid/v2"
)

// Generator produces globally unique, monotonically ordered ULIDs.
type Generator struct {
	mu  sync.Mutex
	ent *ulid.MonotonicEntropy
}

// NewGenerator creates a Generator with a monotonic entropy source.
func NewGenerator() *Generator {
	return &Generator{ent: ulid.Monotonic(nil, 0)}
}

// New returns a new ULID as a canonical 26-character string.
func (g *Generator) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return ulid.Make().String()
}
