package sqltype

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
)

func TestText(t *testing.T) {
	assert.Equal(t, pgtype.Text{String: "value", Valid: true}, Text("value"))
	assert.Equal(t, pgtype.Text{}, Text(""))
	assert.Equal(t, "value", TextValue(pgtype.Text{String: "value", Valid: true}))
	assert.Empty(t, TextValue(pgtype.Text{}))
}

func TestTimestamptz(t *testing.T) {
	value := time.Date(2025, 1, 1, 12, 30, 0, 0, time.FixedZone("offset", -5*60*60))
	want := time.Date(2025, 1, 1, 17, 30, 0, 0, time.UTC)

	assert.Equal(t, pgtype.Timestamptz{Time: want, Valid: true}, Timestamptz(value))
	assert.Equal(t, pgtype.Timestamptz{}, OptionalTimestamptz(value, false))
	assert.Equal(t, want, TimeValue(pgtype.Timestamptz{Time: value, Valid: true}))
	assert.True(t, TimeValue(pgtype.Timestamptz{}).IsZero())
}

func TestInt8(t *testing.T) {
	assert.Equal(t, pgtype.Int8{Int64: 42, Valid: true}, Int8(42))
	assert.Equal(t, pgtype.Int8{}, OptionalInt8(42, false))
}
