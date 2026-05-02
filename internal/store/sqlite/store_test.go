package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/store/sqlite"
)

var errRollback = errors.New("intentional rollback")

func TestOpen_AppliesMigrationsAndDefaultSettings(t *testing.T) {
	t.Parallel()
	s := mustOpenTestDB(t)

	bs, err := s.Bootstrap(context.Background())
	require.NoError(t, err)
	require.Equal(t, "Dinchy", bs.InstanceName)
	require.True(t, bs.SetupRequired)
}

func TestPingContext_Healthy(t *testing.T) {
	t.Parallel()
	s := mustOpenTestDB(t)
	require.NoError(t, s.PingContext(context.Background()))
}

func TestWithTx_CommitsOnSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	_, err := createTestUser(ctx, t, s)
	require.NoError(t, err)

	bs, err := s.Bootstrap(ctx)
	require.NoError(t, err)
	require.False(t, bs.SetupRequired, "setup should not be required after first user is created")
}

func TestWithTx_RollsBackOnError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	err := s.WithTx(ctx, func(_ *sqlite.Store) error {
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)

	bs, err := s.Bootstrap(ctx)
	require.NoError(t, err)
	require.True(t, bs.SetupRequired, "setup should still be required after rollback")
}
