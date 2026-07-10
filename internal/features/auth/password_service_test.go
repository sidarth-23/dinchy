package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

type fakeSender struct {
	configured bool
	sent       []email.Message
}

func (s *fakeSender) Configured() bool { return s.configured }

func (s *fakeSender) Send(_ context.Context, msg email.Message) error {
	s.sent = append(s.sent, msg)
	return nil
}

func newServiceWithSender(t *testing.T, sender email.Sender) (*Service, *MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	mailer, err := email.NewMailer(sender, "https://app.test")
	require.NoError(t, err)
	base := features.ServiceDependencies{Clock: clock.Fixed(fixedTime), IDGenerator: id.NewGenerator()}
	sessionSvc, err := session.NewService(session.Dependencies{Base: base, Store: store, Config: config.DefaultSession(), CacheConfig: config.DefaultCache()})
	require.NoError(t, err)
	svc, err := NewService(Dependencies{Base: base, Store: store, Sessions: sessionSvc, AuthConfig: config.DefaultAuth(), RedisClient: newTestRedis(t), CacheKeyer: cache.NewKeyer("test"), Mailer: mailer})
	require.NoError(t, err)
	svc.beginTx = func(context.Context) (*setupTransaction, error) {
		return &setupTransaction{
			queries:  store,
			commit:   func() error { return nil },
			rollback: func() error { return nil },
		}, nil
	}
	return svc, store
}

func verificationTokenRow(rowID, userID, email, purpose, tokenHash string, expiresAt, consumedAt time.Time, consumedAtValid bool) sqlcgen.FindVerificationTokenRow {
	nullUserID := uuid.NullUUID{}
	if userID != "" {
		nullUserID = uuid.NullUUID{UUID: id.MustParse(userID), Valid: true}
	}
	return sqlcgen.FindVerificationTokenRow{
		ID:         id.MustParse(rowID),
		UserID:     nullUserID,
		Email:      email,
		Purpose:    purpose,
		TokenHash:  tokenHash,
		ExpiresAt:  sqltype.Timestamptz(expiresAt),
		ConsumedAt: sqltype.OptionalTimestamptz(consumedAt, consumedAtValid),
	}
}

func TestForgotPassword_EmailNotConfigured(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t) // NoopSender reports itself unconfigured.

	err := svc.ForgotPassword(testCtx, "user@example.com")
	require.ErrorIs(t, err, apperrors.Internal(i18n.Msg(i18n.CodeEmailNotConfigured)))
}

func TestForgotPassword_UnknownUserIsSilent(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{configured: true}
	svc, store := newServiceWithSender(t, sender)

	store.EXPECT().FindUserByEmail(gomock.Any(), "user@example.com").Return(sqlcgen.FindUserByEmailRow{}, pgx.ErrNoRows)
	// No token created and no mail sent for an unknown address (no user enumeration).

	require.NoError(t, svc.ForgotPassword(testCtx, "user@example.com"))
	assert.Empty(t, sender.sent)
}

func TestForgotPassword_CreatesTokenAndSends(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{configured: true}
	svc, store := newServiceWithSender(t, sender)

	store.EXPECT().FindUserByEmail(gomock.Any(), "user@example.com").
		Return(findUserRow(testUserID, "user@example.com", "User"), nil)
	store.EXPECT().InsertVerificationToken(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, token sqlcgen.InsertVerificationTokenParams) error {
			assert.Equal(t, testUserID, token.UserID.UUID.String())
			assert.True(t, token.UserID.Valid)
			assert.Equal(t, string(VerificationPurposePasswordReset), token.Purpose)
			assert.NotEmpty(t, token.TokenHash)
			assert.True(t, sqltype.TimeValue(token.ExpiresAt).After(fixedTime), "token must expire in the future")
			return nil
		})

	// Email normalization happens at the transport boundary (ForgotPasswordBody.Resolve),
	// so the service receives an already-canonical address.
	require.NoError(t, svc.ForgotPassword(testCtx, "user@example.com"))
	require.Len(t, sender.sent, 1)
	assert.Equal(t, "user@example.com", sender.sent[0].To)
	assert.NotEmpty(t, sender.sent[0].Text)
}

func TestResetPassword_InvalidToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		token sqlcgen.FindVerificationTokenRow
	}{
		{name: "not found", token: sqlcgen.FindVerificationTokenRow{}},
		{name: "expired", token: verificationTokenRow(testVerificationTokenID, testUserID, "user@example.com", string(VerificationPurposePasswordReset), "hash", fixedTime.Add(-time.Minute), time.Time{}, false)},
		{name: "already consumed", token: verificationTokenRow(testVerificationTokenID, testUserID, "user@example.com", string(VerificationPurposePasswordReset), "hash", fixedTime.Add(time.Hour), fixedTime, true)},
		{name: "no associated user", token: verificationTokenRow(testVerificationTokenID, "", "user@example.com", string(VerificationPurposePasswordReset), "hash", fixedTime.Add(time.Hour), time.Time{}, false)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, store := newTestService(t)
			store.EXPECT().
				FindVerificationToken(gomock.Any(), gomock.Any()).
				Return(tc.token, nil)

			err := svc.ResetPassword(testCtx, "raw-token", "newpassword123")
			require.ErrorIs(t, err, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvalidResetToken)))
		})
	}
}

func TestResetPassword_Success(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	token := verificationTokenRow(testVerificationTokenID, testUserID, "user@example.com", string(VerificationPurposePasswordReset), "hash", fixedTime.Add(time.Hour), time.Time{}, false)
	store.EXPECT().
		FindVerificationToken(gomock.Any(), gomock.Any()).
		Return(token, nil)
	store.EXPECT().UpdateUserPasswordHash(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in sqlcgen.UpdateUserPasswordHashParams) error {
			assert.Equal(t, testUserID, in.UserID.String())
			assert.NotEmpty(t, in.PasswordHash.String)
			return nil
		})
	store.EXPECT().ConsumeVerificationToken(gomock.Any(), gomock.Any()).Return(nil)
	// Resetting the password must revoke every existing session for the user.
	store.EXPECT().RevokeSessionsForUser(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, svc.ResetPassword(testCtx, "raw-token", "newpassword123"))
}
