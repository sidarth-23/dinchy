package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/module"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/email"
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

type fakeEnqueuer struct {
	enqueued []river.JobArgs
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, args river.JobArgs, _ *river.InsertOpts) error {
	f.enqueued = append(f.enqueued, args)
	return nil
}

func (f *fakeEnqueuer) EnqueueTx(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) error {
	f.enqueued = append(f.enqueued, args)
	return nil
}

func newServiceWithSender(t *testing.T, sender email.Sender) (*Service, *MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	enqueuer := &fakeEnqueuer{}
	mailer, err := email.NewMailer(enqueuer, sender.Configured())
	require.NoError(t, err)
	sharedService := module.Service{Clock: clock.Fixed(fixedTime), IDGenerator: id.NewGenerator(), RedisClient: newTestRedis(t), CacheKeyer: cache.NewKeyer("test"), Mailer: mailer, Jobs: enqueuer}
	cacheConfig := config.DefaultCache()
	cacheConfig.Enabled = false
	sessionSvc, err := session.NewService(sharedService.Named("session"), store, config.DefaultSession(), cacheConfig)
	require.NoError(t, err)
	svc, err := NewService(sharedService.Named("auth"), store, sessionSvc, config.DefaultAuth(), config.NewLinks("https://app.test"), nil)
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
	require.ErrorIs(t, err, apperrors.Internal(i18n.Msg(i18n.CodeNotificationEmailNotConfigured)))
}

func TestForgotPassword_UnknownUserIsSilent(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{configured: true}
	svc, store := newServiceWithSender(t, sender)

	store.EXPECT().FindUserByEmail(gomock.Any(), "user@example.com").Return(sqlcgen.FindUserByEmailRow{}, pgx.ErrNoRows)
	// No token created and no mail sent for an unknown address (no user enumeration).

	require.NoError(t, svc.ForgotPassword(testCtx, "user@example.com"))
	assert.Empty(t, svc.Jobs.(*fakeEnqueuer).enqueued, "no email enqueued for an unknown address")
}

func TestForgotPassword_CreatesTokenAndEnqueuesEmail(t *testing.T) {
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
	// so the service receives an already-canonical address. Delivery is durable: the mailer
	// enqueues the reset email so a transient SMTP failure is retried rather than lost.
	require.NoError(t, svc.ForgotPassword(testCtx, "user@example.com"))

	enqueuer := svc.Jobs.(*fakeEnqueuer)
	require.Len(t, enqueuer.enqueued, 1)
	args, ok := enqueuer.enqueued[0].(email.SendEmailArgs)
	require.True(t, ok)
	assert.Equal(t, "user@example.com", args.To)
	assert.Contains(t, args.Text, "/reset-password?token=", "reset email must carry the reset link")
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
			require.ErrorIs(t, err, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthInvalidResetToken)))
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
