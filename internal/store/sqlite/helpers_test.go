package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/store/sqlite"
	"github.com/sidarth-23/dinchy/internal/testutil"
)

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
