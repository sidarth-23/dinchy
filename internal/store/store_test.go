package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/store"
	"github.com/sidarth-23/dinchy/internal/store/testsupport"
	"github.com/sidarth-23/dinchy/internal/store/types"
)

var fixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	return testsupport.OpenPostgresStore(t)
}

func seedFirstUser(t *testing.T, db *store.Store) auth.CreateUserInput {
	t.Helper()

	in := auth.CreateUserInput{
		ID:                   "00000000-0000-0000-0000-000000000001",
		AccountID:            "00000000-0000-0000-0000-000000000002",
		OrganisationID:       "00000000-0000-0000-0000-000000000003",
		OrganisationMemberID: "00000000-0000-0000-0000-000000000004",
		Email:                "owner@example.com",
		PasswordHash:         "hash",
		DisplayName:          "Owner",
		OrganisationName:     "Dinchy",
		OrganisationSlug:     "dinchy",
		Now:                  fixedTime,
	}

	user, err := db.CreateFirstUser(t.Context(), in)
	require.NoError(t, err)
	assert.Equal(t, in.ID, user.ID)
	return in
}

func TestStoreBootstrap(t *testing.T) {
	store := newTestStore(t)

	state, err := store.Bootstrap(t.Context())
	require.NoError(t, err)
	assert.True(t, state.SetupRequired)
	assert.Equal(t, "Dinchy", state.InstanceName)

	seedFirstUser(t, store)

	state, err = store.Bootstrap(t.Context())
	require.NoError(t, err)
	assert.False(t, state.SetupRequired)
	assert.Equal(t, "Dinchy", state.InstanceName)
}

func TestStoreUserAndSessionRoundTrip(t *testing.T) {
	store := newTestStore(t)
	in := seedFirstUser(t, store)

	user, err := store.FindUserByEmail(t.Context(), in.Email)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, in.Email, user.Email)

	account, err := store.FindPasswordAccountByUserID(t.Context(), in.ID)
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, in.AccountID, account.ID)

	orgs, err := store.ListOrganisationsForUser(t.Context(), in.ID)
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	assert.Equal(t, in.OrganisationSlug, orgs[0].Slug)

	session, err := store.CreateSession(t.Context(), auth.CreateSessionInput{
		ID:             "session-1",
		UserID:         in.ID,
		OrganisationID: in.OrganisationID,
		TokenHash:      "token-1",
		IP:             "127.0.0.1",
		UserAgent:      "test-agent",
		Now:            fixedTime,
		IdleExpiresAt:  fixedTime.Add(time.Minute),
		ExpiresAt:      fixedTime.Add(time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, "session-1", session.ID)

	loaded, err := store.GetSessionByTokenHash(t.Context(), "token-1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, in.Email, loaded.Email)
	assert.Equal(t, in.OrganisationSlug, loaded.OrganisationSlug)

	require.NoError(t, store.RevokeSessionByTokenHash(t.Context(), "token-1"))

	revoked, err := store.GetSessionByTokenHash(t.Context(), "token-1")
	require.NoError(t, err)
	require.NotNil(t, revoked)
	assert.True(t, revoked.RevokedAt.Valid)

	deleted, err := store.DeleteEndedSessionsOlderThan(t.Context(), fixedTime.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	ended, err := store.GetSessionByTokenHash(t.Context(), "token-1")
	require.NoError(t, err)
	assert.Nil(t, ended)
}

func TestStoreTaskLifecycle(t *testing.T) {
	store := newTestStore(t)

	require.NoError(t, store.EnsureTask(t.Context(), "session_cleanup", 300, fixedTime))

	claimed, err := store.ClaimTask(t.Context(), "session_cleanup", "owner-a", fixedTime.Add(15*time.Second), fixedTime)
	require.NoError(t, err)
	assert.True(t, claimed)

	contended, err := store.ClaimTask(t.Context(), "session_cleanup", "owner-b", fixedTime.Add(30*time.Second), fixedTime.Add(5*time.Second))
	require.NoError(t, err)
	assert.False(t, contended)

	nextRun := fixedTime.Add(5 * time.Minute)
	require.NoError(t, store.FinishTask(t.Context(), "session_cleanup", fixedTime, true, "", "", nextRun))

	tooEarly, err := store.ClaimTask(t.Context(), "session_cleanup", "owner-a", fixedTime.Add(2*time.Minute), fixedTime.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, tooEarly)

	due, err := store.ClaimTask(t.Context(), "session_cleanup", "owner-a", nextRun.Add(15*time.Second), nextRun.Add(time.Second))
	require.NoError(t, err)
	assert.True(t, due)
}

func TestStoreAuditLogRoundTrip(t *testing.T) {
	store := newTestStore(t)

	now := fixedTime
	require.NoError(t, store.InsertAuditLog(t.Context(), types.InsertAuditLogParams{
		ID:          "00000000-0000-0000-0000-00000000000a",
		Category:    "security",
		Subcategory: "auth",
		EventType:   "login",
		Action:      "login",
		Outcome:     "succeeded",
		IPAddress:   "127.0.0.1",
		UserAgent:   "test-agent",
		CreatedAt:   now,
	}))

	rows, err := store.ListAuditLogs(t.Context(), types.ListAuditLogsParams{
		Category:    "security",
		Subcategory: "auth",
		EventType:   "login",
		Outcome:     "succeeded",
		Before:      now.Add(time.Minute),
		BeforeValid: true,
		Limit:       10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "login", rows[0].Action)
}
