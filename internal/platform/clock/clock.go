// Package clock provides a mockable time source for testability.
package clock

import "time"

// Clock abstracts time so it can be controlled in tests.
type Clock interface {
	Now() time.Time
}

// RealClock returns the current UTC time using the system clock.
type RealClock struct{}

// Now returns the current time in UTC.
func (RealClock) Now() time.Time { return time.Now().UTC() }
