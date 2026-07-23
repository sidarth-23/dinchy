package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPasswordAndVerifyPassword(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("secret")
	require.NoError(t, err)
	assert.True(t, VerifyPassword("secret", hash))
	assert.False(t, VerifyPassword("wrong", hash))
}
