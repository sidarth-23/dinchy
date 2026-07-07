// Package sqltype converts Go values to and from pgx nullable column types.
package sqltype

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Text wraps a string as a pgtype.Text, treating the empty string as SQL NULL.
func Text(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

// OptionalText wraps a string as a pgtype.Text, using SQL NULL when valid is false.
func OptionalText(value string, valid bool) pgtype.Text {
	if !valid {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

// TextValue returns the string held by a pgtype.Text, or the empty string when NULL.
func TextValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// Timestamptz wraps a time as a non-NULL pgtype.Timestamptz normalized to UTC.
func Timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

// OptionalTimestamptz wraps a time as a pgtype.Timestamptz, using SQL NULL when valid is false.
func OptionalTimestamptz(value time.Time, valid bool) pgtype.Timestamptz {
	if !valid {
		return pgtype.Timestamptz{}
	}
	return Timestamptz(value)
}

// TimeValue returns the UTC time held by a pgtype.Timestamptz, or the zero time when NULL.
func TimeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

// Int8 wraps an int64 as a non-NULL pgtype.Int8.
func Int8(value int64) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: true}
}

// OptionalInt8 wraps an int64 as a pgtype.Int8, using SQL NULL when valid is false.
func OptionalInt8(value int64, valid bool) pgtype.Int8 {
	if !valid {
		return pgtype.Int8{}
	}
	return Int8(value)
}
