package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootstrap_NoUsers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	bs, err := s.Bootstrap(ctx)
	require.NoError(t, err)
	assert.True(t, bs.SetupRequired)
	assert.Equal(t, "Dinchy", bs.InstanceName)
}

func TestBootstrap_WithUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	_, err := createTestUser(ctx, t, s)
	require.NoError(t, err)

	bs, err := s.Bootstrap(ctx)
	require.NoError(t, err)
	assert.False(t, bs.SetupRequired)
	assert.Equal(t, "Dinchy", bs.InstanceName)
}
