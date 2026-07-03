package sqlite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Bootstrap_SetupRequiredOnEmptyInstance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	state, err := store.Bootstrap(testCtx)
	require.NoError(t, err)
	assert.True(t, state.SetupRequired)
	// Open seeds the default settings row, so the instance name is available.
	assert.Equal(t, "Dinchy", state.InstanceName)
}

func TestStore_Bootstrap_SetupCompleteAfterFirstUser(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	seedFirstUser(t, store)

	state, err := store.Bootstrap(testCtx)
	require.NoError(t, err)
	assert.False(t, state.SetupRequired)
	assert.Equal(t, "Dinchy", state.InstanceName)
}
