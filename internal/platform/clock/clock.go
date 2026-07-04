// Package clock provides a mockable time source for testability.
package clock

import (
	"database/sql"
	"time"
)

// Clock abstracts time so it can be controlled in tests.
type Clock interface {
	Now() time.Time
}

// RealClock returns the current UTC time using the system clock.
type RealClock struct{}

// Now returns the current time in UTC.
func (RealClock) Now() time.Time { return time.Now().UTC() }

// UTC normalizes a time to UTC.
func UTC(value time.Time) time.Time {
	return value.UTC()
}

// NullTime converts a time and validity flag into a nullable SQL time.
func NullTime(value time.Time, valid bool) sql.NullTime {
	if !valid {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}
