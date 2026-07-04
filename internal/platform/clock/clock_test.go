package clock

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUTC(t *testing.T) {
	loc := time.FixedZone("offset", 2*60*60)
	value := time.Date(2025, 1, 1, 12, 30, 0, 0, loc)

	assert.Equal(t, time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC), UTC(value))
}

func TestNullTime(t *testing.T) {
	value := time.Date(2025, 1, 1, 12, 30, 0, 0, time.FixedZone("offset", -5*60*60))

	assert.Equal(t, sql.NullTime{Time: time.Date(2025, 1, 1, 17, 30, 0, 0, time.UTC), Valid: true}, NullTime(value, true))
	assert.Equal(t, sql.NullTime{}, NullTime(value, false))
}
