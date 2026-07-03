package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite"
)

var testCtx = context.Background()

// fixedTime carries a non-zero nanosecond component so the RFC3339Nano
// format/parse round-trip in the adapter is exercised, not just second precision.
var fixedTime = time.Date(2025, 1, 1, 12, 0, 0, 123456789, time.UTC)

func newTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	return core.OpenTestDB(t, sqlite.Open, filepath.Join(t.TempDir(), "test.db"))
}

// seedFirstUser creates the first user, account, organisation, and owner
// membership, returning the input so tests can reuse its identifiers.
func seedFirstUser(t *testing.T, store *sqlite.Store) auth.CreateUserInput {
	t.Helper()
	in := auth.CreateUserInput{
		ID:                   "user-1",
		AccountID:            "account-1",
		OrganisationID:       "org-1",
		OrganisationMemberID: "member-1",
		Email:                "owner@example.com",
		PasswordHash:         "hash-1",
		DisplayName:          "Owner",
		OrganisationName:     "Acme",
		OrganisationSlug:     "acme",
		Now:                  fixedTime,
	}
	user, err := store.CreateFirstUser(testCtx, in)
	require.NoError(t, err)
	require.Equal(t, in.ID, user.ID)
	return in
}

func TestStore_CreateFirstUser_RoundTrip(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	in := seedFirstUser(t, store)

	user, err := store.FindUserByEmail(testCtx, in.Email)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, in.ID, user.ID)
	assert.Equal(t, in.DisplayName, user.DisplayName)

	account, err := store.FindPasswordAccountByUserID(testCtx, in.ID)
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, in.PasswordHash, account.PasswordHash)
	assert.Equal(t, string(auth.AccountProviderPassword), account.Provider)

	orgs, err := store.ListOrganisationsForUser(testCtx, in.ID)
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	assert.Equal(t, in.OrganisationSlug, orgs[0].Slug)
	assert.Equal(t, auth.RoleOwner, orgs[0].Role)

	bySlug, err := store.FindOrganisationBySlugForUser(testCtx, in.ID, in.OrganisationSlug)
	require.NoError(t, err)
	require.NotNil(t, bySlug)
	assert.Equal(t, in.OrganisationID, bySlug.ID)

	byID, err := store.FindOrganisationByIDForUser(testCtx, in.ID, in.OrganisationID)
	require.NoError(t, err)
	require.NotNil(t, byID)
	assert.Equal(t, in.OrganisationSlug, byID.Slug)
}

func TestStore_CreateFirstUser_SecondAttemptConflicts(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	seedFirstUser(t, store)

	second := auth.CreateUserInput{
		ID:                   "user-2",
		AccountID:            "account-2",
		OrganisationID:       "org-2",
		OrganisationMemberID: "member-2",
		Email:                "second@example.com",
		PasswordHash:         "hash-2",
		DisplayName:          "Second",
		OrganisationName:     "Beta",
		OrganisationSlug:     "beta",
		Now:                  fixedTime,
	}
	_, err := store.CreateFirstUser(testCtx, second)
	require.ErrorIs(t, err, apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted)))

	// The rejected setup must not have leaked a partial user.
	leaked, err := store.FindUserByEmail(testCtx, second.Email)
	require.NoError(t, err)
	assert.Nil(t, leaked)
}

func TestStore_FindUserByEmail_NotFoundReturnsNil(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user, err := store.FindUserByEmail(testCtx, "missing@example.com")
	require.NoError(t, err)
	assert.Nil(t, user)
}

func TestStore_FindUserByProviderAccount(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	in := seedFirstUser(t, store)

	user, err := store.FindUserByProviderAccount(testCtx, string(auth.AccountProviderPassword), in.Email)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, in.ID, user.ID)

	missing, err := store.FindUserByProviderAccount(testCtx, string(auth.AccountProviderPassword), "nobody@example.com")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestStore_UpdateUserPasswordHash(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	in := seedFirstUser(t, store)

	require.NoError(t, store.UpdateUserPasswordHash(testCtx, auth.UpdateUserPasswordHashInput{
		UserID:       in.ID,
		PasswordHash: "rotated-hash",
		Now:          fixedTime.Add(time.Minute),
	}))

	account, err := store.FindPasswordAccountByUserID(testCtx, in.ID)
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, "rotated-hash", account.PasswordHash)
}

func TestStore_Session_Lifecycle(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	in := seedFirstUser(t, store)

	idleExpiry := fixedTime.Add(30 * time.Minute)
	absoluteExpiry := fixedTime.Add(7 * 24 * time.Hour)
	_, err := store.CreateSession(testCtx, auth.CreateSessionInput{
		ID:             "session-1",
		UserID:         in.ID,
		OrganisationID: in.OrganisationID,
		TokenHash:      "token-1",
		IP:             "127.0.0.1",
		UserAgent:      "test-agent",
		Now:            fixedTime,
		IdleExpiresAt:  idleExpiry,
		ExpiresAt:      absoluteExpiry,
	})
	require.NoError(t, err)

	sess, err := store.GetSessionByTokenHash(testCtx, "token-1")
	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.Equal(t, in.ID, sess.UserID)
	assert.Equal(t, in.Email, sess.Email)
	assert.Equal(t, in.OrganisationSlug, sess.OrganisationSlug)
	assert.Equal(t, auth.RoleOwner, sess.Role)
	assert.False(t, sess.RevokedAt.Valid)
	// Timestamps must survive the format/parse round-trip at nanosecond precision.
	assert.True(t, idleExpiry.Equal(sess.IdleExpiresAt), "idle expiry: want %s got %s", idleExpiry, sess.IdleExpiresAt)
	assert.True(t, absoluteExpiry.Equal(sess.ExpiresAt), "absolute expiry: want %s got %s", absoluteExpiry, sess.ExpiresAt)

	require.NoError(t, store.RevokeSessionByTokenHash(testCtx, "token-1"))
	revoked, err := store.GetSessionByTokenHash(testCtx, "token-1")
	require.NoError(t, err)
	require.NotNil(t, revoked)
	assert.True(t, revoked.RevokedAt.Valid)
}

func TestStore_GetSessionByTokenHash_NotFoundReturnsNil(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	seedFirstUser(t, store)

	sess, err := store.GetSessionByTokenHash(testCtx, "no-such-token")
	require.NoError(t, err)
	assert.Nil(t, sess)
}

func TestStore_RevokeSessionsForUser(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	in := seedFirstUser(t, store)

	for _, tokenHash := range []string{"token-a", "token-b"} {
		_, err := store.CreateSession(testCtx, auth.CreateSessionInput{
			ID:             "session-" + tokenHash,
			UserID:         in.ID,
			OrganisationID: in.OrganisationID,
			TokenHash:      tokenHash,
			IP:             "127.0.0.1",
			UserAgent:      "test-agent",
			Now:            fixedTime,
			IdleExpiresAt:  fixedTime.Add(30 * time.Minute),
			ExpiresAt:      fixedTime.Add(7 * 24 * time.Hour),
		})
		require.NoError(t, err)
	}

	require.NoError(t, store.RevokeSessionsForUser(testCtx, in.ID, fixedTime.Add(time.Minute)))

	for _, tokenHash := range []string{"token-a", "token-b"} {
		sess, err := store.GetSessionByTokenHash(testCtx, tokenHash)
		require.NoError(t, err)
		require.NotNil(t, sess)
		assert.True(t, sess.RevokedAt.Valid, "session %q should be revoked", tokenHash)
	}
}

func TestStore_VerificationToken_Flow(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	in := seedFirstUser(t, store)

	expiry := fixedTime.Add(time.Hour)
	require.NoError(t, store.CreateVerificationToken(testCtx, auth.VerificationToken{
		ID:          "vtoken-1",
		UserID:      in.ID,
		UserIDValid: true,
		Email:       in.Email,
		Purpose:     string(auth.VerificationPurposePasswordReset),
		TokenHash:   "vhash-1",
		ExpiresAt:   expiry,
	}))

	token, err := store.FindVerificationToken(testCtx, "vhash-1", string(auth.VerificationPurposePasswordReset))
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, in.ID, token.UserID)
	assert.True(t, token.UserIDValid)
	assert.False(t, token.ConsumedAtValid)
	assert.True(t, expiry.Equal(token.ExpiresAt), "expiry: want %s got %s", expiry, token.ExpiresAt)

	consumedAt := fixedTime.Add(10 * time.Minute)
	require.NoError(t, store.ConsumeVerificationToken(testCtx, token.ID, consumedAt))

	reused, err := store.FindVerificationToken(testCtx, "vhash-1", string(auth.VerificationPurposePasswordReset))
	require.NoError(t, err)
	require.NotNil(t, reused)
	assert.True(t, reused.ConsumedAtValid)
	assert.True(t, consumedAt.Equal(reused.ConsumedAt), "consumed at: want %s got %s", consumedAt, reused.ConsumedAt)
}

func TestStore_FindVerificationToken_NotFoundReturnsNil(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	token, err := store.FindVerificationToken(testCtx, "absent", string(auth.VerificationPurposePasswordReset))
	require.NoError(t, err)
	assert.Nil(t, token)
}

func TestStore_TwoFactor_Lifecycle(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	in := seedFirstUser(t, store)

	require.NoError(t, store.SaveTwoFactor(testCtx, auth.TwoFactor{
		ID:       "tf-1",
		UserID:   in.ID,
		Secret:   "SECRETSEED",
		Verified: false,
	}))

	pending, err := store.FindTwoFactorByUserID(testCtx, in.ID)
	require.NoError(t, err)
	require.NotNil(t, pending)
	assert.Equal(t, "SECRETSEED", pending.Secret)
	assert.False(t, pending.Verified)
	assert.False(t, pending.LastUsedStepValid)
	assert.False(t, pending.LockedUntilValid)
	assert.Zero(t, pending.FailedVerificationCount)

	require.NoError(t, store.ConfirmTwoFactor(testCtx, in.ID, 42, fixedTime.Add(time.Minute)))
	confirmed, err := store.FindTwoFactorByUserID(testCtx, in.ID)
	require.NoError(t, err)
	require.NotNil(t, confirmed)
	assert.True(t, confirmed.Verified)
	assert.True(t, confirmed.LastUsedStepValid)
	assert.Equal(t, int64(42), confirmed.LastUsedStep)

	require.NoError(t, store.MarkTwoFactorUsed(testCtx, in.ID, 43, fixedTime.Add(2*time.Minute)))
	used, err := store.FindTwoFactorByUserID(testCtx, in.ID)
	require.NoError(t, err)
	require.NotNil(t, used)
	assert.True(t, used.Verified)
	assert.Equal(t, int64(43), used.LastUsedStep)

	require.NoError(t, store.DisableTwoFactor(testCtx, in.ID))
	disabled, err := store.FindTwoFactorByUserID(testCtx, in.ID)
	require.NoError(t, err)
	assert.Nil(t, disabled)
}

func TestStore_SSOProviderSettings_Upsert(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.UpsertSSOProviderSetting(testCtx, auth.UpsertSSOProviderSettingInput{
		ProviderID:    "google",
		ClientID:      "client-id",
		ClientIDValid: true,
		Secret:        "client-secret",
		SecretValid:   true,
		CallbackURL:   "https://example.test/callback",
		CallbackValid: true,
		Enabled:       true,
		Now:           fixedTime,
	}))

	settings, err := store.ListSSOProviderSettings(testCtx)
	require.NoError(t, err)
	require.Len(t, settings, 1)
	assert.Equal(t, "google", settings[0].ProviderID)
	assert.Equal(t, "client-id", settings[0].ClientID)
	assert.True(t, settings[0].ClientIDValid)
	assert.True(t, settings[0].Enabled)

	// Upserting the same provider updates in place and clears the nullable columns.
	require.NoError(t, store.UpsertSSOProviderSetting(testCtx, auth.UpsertSSOProviderSettingInput{
		ProviderID:    "google",
		ClientIDValid: false,
		SecretValid:   false,
		CallbackValid: false,
		Enabled:       false,
		Now:           fixedTime.Add(time.Hour),
	}))

	updated, err := store.ListSSOProviderSettings(testCtx)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.False(t, updated[0].ClientIDValid)
	assert.Empty(t, updated[0].ClientID)
	assert.False(t, updated[0].Enabled)
}
