package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/store/sqlite"
	"github.com/sidarth-23/dinchy/internal/testutil"
)

var errRollback = errors.New("intentional rollback")

func createTestUser(ctx context.Context, t testing.TB, s *sqlite.Store) (auth.User, error) {
	t.Helper()
	return s.CreateFirstUser(ctx, auth.CreateUserInput{
		ID:           "01J0000000000000000000001",
		Email:        "admin@example.com",
		PasswordHash: "hashedpassword",
		DisplayName:  "Admin",
		Now:          time.Now().UTC(),
	})
}

func createTestSession(ctx context.Context, t testing.TB, s *sqlite.Store, userID string) session.Session {
	t.Helper()
	now := time.Now().UTC()
	sess, err := s.CreateSession(ctx, session.CreateSessionInput{
		ID:            "01J0000000000000000000002",
		UserID:        userID,
		TokenHash:     "tokenhashabcdef",
		IP:            "127.0.0.1",
		UserAgent:     "test-agent",
		Now:           now,
		IdleExpiresAt: now.Add(30 * time.Minute),
		ExpiresAt:     now.Add(7 * 24 * time.Hour),
	})
	require.NoError(t, err)
	return sess
}

func mustOpenTestDB(t testing.TB) *sqlite.Store {
	t.Helper()
	return testutil.OpenTestDB(t)
}

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
