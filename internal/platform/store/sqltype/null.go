package sqltype

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func Text(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func OptionalText(value string, valid bool) pgtype.Text {
	if !valid {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func TextValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func Timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func OptionalTimestamptz(value time.Time, valid bool) pgtype.Timestamptz {
	if !valid {
		return pgtype.Timestamptz{}
	}
	return Timestamptz(value)
}

func TimeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func Int8(value int64) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: true}
}

func OptionalInt8(value int64, valid bool) pgtype.Int8 {
	if !valid {
		return pgtype.Int8{}
	}
	return Int8(value)
}
