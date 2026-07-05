package security

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomToken(t *testing.T) {
	t.Parallel()
	token, err := RandomToken(32)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	_, err = base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
}

func TestHashToken(t *testing.T) {
	t.Parallel()
	assert.Equal(t, HashToken("raw"), HashToken("raw"))
	assert.NotEqual(t, HashToken("raw"), HashToken("other"))
}
