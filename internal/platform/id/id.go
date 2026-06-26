// Package id provides UUIDv7 generation.
package id

import (
	"github.com/google/uuid"
)

// Generator produces globally unique UUIDv7 identifiers.
type Generator struct{}

// NewGenerator creates a Generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// New returns a new UUIDv7 as a canonical string.
func (g *Generator) New() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id.String()
}
