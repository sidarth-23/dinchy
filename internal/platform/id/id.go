// Package id provides UUIDv7 generation.
package id

import (
	"fmt"

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

// Parse converts a canonical UUID string into a UUID value.
func Parse(value string) (uuid.UUID, error) {
	return uuid.Parse(value)
}

// UUIDField identifies one UUID input by key and raw value.
type UUIDField struct {
	Key   string
	Value string
}

// ParseFields converts labeled UUID strings into UUID values in order.
func ParseFields(fields ...UUIDField) ([]uuid.UUID, error) {
	values := make([]uuid.UUID, 0, len(fields))
	for _, field := range fields {
		parsed, err := Parse(field.Value)
		if err != nil {
			return nil, fmt.Errorf("parse uuid for %s=%q: %w", field.Key, field.Value, err)
		}
		values = append(values, parsed)
	}
	return values, nil
}

// NullUUID converts a string and validity flag into a nullable UUID wrapper.
func NullUUID(value string, valid bool) (uuid.NullUUID, error) {
	if !valid {
		return uuid.NullUUID{}, nil
	}
	parsed, err := Parse(value)
	if err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: parsed, Valid: true}, nil
}
