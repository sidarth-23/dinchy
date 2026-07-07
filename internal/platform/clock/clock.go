// Package clock provides a mockable time source for testability.
package clock

import "time"

// Clock reports the current time. Production uses the system clock; tests inject a fixed one.
type Clock interface {
	Now() time.Time
}

// System reports wall-clock time in UTC.
type System struct{}

// Now returns the current time in UTC.
func (System) Now() time.Time { return time.Now().UTC() }

// Fixed returns a Clock that always reports t. Intended for tests.
func Fixed(t time.Time) Clock { return fixed{t: t} }

type fixed struct{ t time.Time }

func (f fixed) Now() time.Time { return f.t }
