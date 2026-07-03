package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cachecore "github.com/sidarth-23/dinchy/internal/cache/core"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
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
	svc, err := NewService(store, id.NewGenerator(), fakeClock{now: fixedTime}, config.DefaultAuth(), nil, newTestCache(), cachecore.NewKeyer("test"), sender)
	require.NoError(t, err)
	return svc, store
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

	store.EXPECT().FindUserByEmail(gomock.Any(), "user@example.com").Return(nil, nil)
	// No token created and no mail sent for an unknown address (no user enumeration).

	require.NoError(t, svc.ForgotPassword(testCtx, "user@example.com"))
	assert.Empty(t, sender.sent)
}

func TestForgotPassword_CreatesTokenAndSends(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{configured: true}
	svc, store := newServiceWithSender(t, sender)

	store.EXPECT().FindUserByEmail(gomock.Any(), "user@example.com").
		Return(&User{ID: "u1", Email: "user@example.com"}, nil)
	store.EXPECT().CreateVerificationToken(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, token VerificationToken) error {
			assert.Equal(t, "u1", token.UserID)
			assert.True(t, token.UserIDValid)
			assert.Equal(t, string(VerificationPurposePasswordReset), token.Purpose)
			assert.NotEmpty(t, token.TokenHash)
			assert.True(t, token.ExpiresAt.After(fixedTime), "token must expire in the future")
			return nil
		})

	// Leading/trailing whitespace and case must be normalized before lookup.
	require.NoError(t, svc.ForgotPassword(testCtx, "  USER@EXAMPLE.COM  "))
	require.Len(t, sender.sent, 1)
	assert.Equal(t, "user@example.com", sender.sent[0].To)
	assert.NotEmpty(t, sender.sent[0].Text)
}

func TestResetPassword_InvalidToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		token *VerificationToken
	}{
		{name: "not found", token: nil},
		{name: "expired", token: &VerificationToken{UserID: "u1", UserIDValid: true, ExpiresAt: fixedTime.Add(-time.Minute)}},
		{name: "already consumed", token: &VerificationToken{UserID: "u1", UserIDValid: true, ExpiresAt: fixedTime.Add(time.Hour), ConsumedAtValid: true}},
		{name: "no associated user", token: &VerificationToken{UserID: "", UserIDValid: false, ExpiresAt: fixedTime.Add(time.Hour)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, store := newTestService(t)
			store.EXPECT().
				FindVerificationToken(gomock.Any(), gomock.Any(), string(VerificationPurposePasswordReset)).
				Return(tc.token, nil)

			err := svc.ResetPassword(testCtx, "raw-token", "newpassword123")
			require.ErrorIs(t, err, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvalidResetToken)))
		})
	}
}

func TestResetPassword_Success(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	token := &VerificationToken{
		ID:          "vt-1",
		UserID:      "u1",
		UserIDValid: true,
		Purpose:     string(VerificationPurposePasswordReset),
		ExpiresAt:   fixedTime.Add(time.Hour),
	}
	store.EXPECT().
		FindVerificationToken(gomock.Any(), gomock.Any(), string(VerificationPurposePasswordReset)).
		Return(token, nil)
	store.EXPECT().UpdateUserPasswordHash(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in UpdateUserPasswordHashInput) error {
			assert.Equal(t, "u1", in.UserID)
			assert.NotEmpty(t, in.PasswordHash)
			return nil
		})
	store.EXPECT().ConsumeVerificationToken(gomock.Any(), "vt-1", fixedTime).Return(nil)
	// Resetting the password must revoke every existing session for the user.
	store.EXPECT().RevokeSessionsForUser(gomock.Any(), "u1", fixedTime).Return(nil)

	require.NoError(t, svc.ResetPassword(testCtx, "raw-token", "newpassword123"))
}
