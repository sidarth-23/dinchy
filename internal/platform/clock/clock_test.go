package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUTC(t *testing.T) {
	loc := time.FixedZone("offset", 2*60*60)
	value := time.Date(2025, 1, 1, 12, 30, 0, 0, loc)

	assert.Equal(t, time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC), UTC(value))
}
