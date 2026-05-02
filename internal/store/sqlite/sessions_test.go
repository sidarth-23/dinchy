package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSession_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	u, err := createTestUser(ctx, t, s)
	require.NoError(t, err)

	sess := createTestSession(ctx, t, s, u.ID)
	assert.NotEmpty(t, sess.ID)
}

func TestGetSessionByTokenHash_Found(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	u, err := createTestUser(ctx, t, s)
	require.NoError(t, err)
	createTestSession(ctx, t, s, u.ID)

	got, err := s.GetSessionByTokenHash(ctx, "tokenhashabcdef")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, u.ID, got.UserID)
	assert.Equal(t, "admin@example.com", got.Email)
	assert.Equal(t, "Admin", got.DisplayName)
	assert.False(t, got.RevokedAt.Valid)
}

func TestGetSessionByTokenHash_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	got, err := s.GetSessionByTokenHash(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestRevokeSessionByTokenHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	u, err := createTestUser(ctx, t, s)
	require.NoError(t, err)
	createTestSession(ctx, t, s, u.ID)

	require.NoError(t, s.RevokeSessionByTokenHash(ctx, "tokenhashabcdef"))

	got, err := s.GetSessionByTokenHash(ctx, "tokenhashabcdef")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.RevokedAt.Valid, "session should be revoked")
}

func TestRevokeSessionByTokenHash_Idempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	u, err := createTestUser(ctx, t, s)
	require.NoError(t, err)
	createTestSession(ctx, t, s, u.ID)

	require.NoError(t, s.RevokeSessionByTokenHash(ctx, "tokenhashabcdef"))
	require.NoError(t, s.RevokeSessionByTokenHash(ctx, "tokenhashabcdef"), "revoking twice should not error")
}

func TestDeleteEndedSessionsOlderThan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	u, err := createTestUser(ctx, t, s)
	require.NoError(t, err)
	createTestSession(ctx, t, s, u.ID)

	// Revoke the session so it qualifies for deletion.
	require.NoError(t, s.RevokeSessionByTokenHash(ctx, "tokenhashabcdef"))

	// Delete sessions older than far future — everything qualifies.
	n, err := s.DeleteEndedSessionsOlderThan(ctx, time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// Verify it's gone.
	got, err := s.GetSessionByTokenHash(ctx, "tokenhashabcdef")
	require.NoError(t, err)
	assert.Nil(t, got)
}
