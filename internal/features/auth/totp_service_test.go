package auth

import (
	"context"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// testTOTPSecret is a valid base32 TOTP seed used to derive live codes.
const testTOTPSecret = "JBSWY3DPEHPK3PXP"

// validTOTPCode generates a code for the current wall clock. The service
// validates codes with totp.Validate, which reads time.Now() directly and is
// not driven by the injected clock, so the code must match real time (the
// recorded step is what uses the fake clock).
func validTOTPCode(t *testing.T) string {
	t.Helper()
	code, err := totp.GenerateCode(testTOTPSecret, time.Now())
	require.NoError(t, err)
	return code
}

func TestStartTOTPEnrollment_SavesUnverifiedSecret(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().SaveTwoFactor(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, tf TwoFactor) error {
			assert.Equal(t, "u1", tf.UserID)
			assert.False(t, tf.Verified, "a freshly enrolled secret must not be verified yet")
			assert.NotEmpty(t, tf.Secret)
			return nil
		})

	secret, url, err := svc.StartTOTPEnrollment(testCtx, "u1", "user@example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, secret)
	assert.NotEmpty(t, url)
}

func TestConfirmTOTP_NoEnrollment(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().FindTwoFactorByUserID(gomock.Any(), "u1").Return(nil, nil)

	err := svc.ConfirmTOTP(testCtx, "u1", "123456")
	require.ErrorIs(t, err, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP)))
}

func TestConfirmTOTP_InvalidCode(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().FindTwoFactorByUserID(gomock.Any(), "u1").
		Return(&TwoFactor{Secret: testTOTPSecret}, nil)

	err := svc.ConfirmTOTP(testCtx, "u1", "000000")
	require.ErrorIs(t, err, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP)))
}

func TestConfirmTOTP_Success(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().FindTwoFactorByUserID(gomock.Any(), "u1").
		Return(&TwoFactor{Secret: testTOTPSecret}, nil)
	store.EXPECT().ConfirmTwoFactor(gomock.Any(), "u1", totpStep(fixedTime), fixedTime).Return(nil)

	require.NoError(t, svc.ConfirmTOTP(testCtx, "u1", validTOTPCode(t)))
}

func TestDisableTOTP_DelegatesToStore(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().DisableTwoFactor(gomock.Any(), "u1").Return(nil)

	require.NoError(t, svc.DisableTOTP(testCtx, "u1"))
}

// expectPasswordLogin wires the credential-verification stage of Login so tests
// can focus on the TOTP branch that follows.
func expectPasswordLogin(t *testing.T, store *MockStore) {
	t.Helper()
	store.EXPECT().FindUserByEmail(gomock.Any(), "user@example.com").
		Return(&User{ID: "u1", Email: "user@example.com"}, nil)
	store.EXPECT().FindPasswordAccountByUserID(gomock.Any(), "u1").
		Return(&Account{UserID: "u1", PasswordHash: HashPasswordForTest(t, "secret")}, nil)
}

func TestLogin_TOTPRequiredWhenCodeMissing(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)
	expectPasswordLogin(t, store)
	store.EXPECT().FindTwoFactorByUserID(gomock.Any(), "u1").
		Return(&TwoFactor{Secret: testTOTPSecret, Verified: true}, nil)

	_, err := svc.Login(testCtx, "user@example.com", "secret", "", "", "", "")
	require.ErrorIs(t, err, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthTOTPRequired)))
}

func TestLogin_TOTPReplayedStepRejected(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)
	expectPasswordLogin(t, store)
	store.EXPECT().FindTwoFactorByUserID(gomock.Any(), "u1").Return(&TwoFactor{
		Secret:            testTOTPSecret,
		Verified:          true,
		LastUsedStep:      totpStep(fixedTime),
		LastUsedStepValid: true,
	}, nil)

	_, err := svc.Login(testCtx, "user@example.com", "secret", "", validTOTPCode(t), "", "")
	require.ErrorIs(t, err, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP)))
}

func TestLogin_TOTPSuccessMarksStepUsed(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)
	expectPasswordLogin(t, store)
	store.EXPECT().FindTwoFactorByUserID(gomock.Any(), "u1").
		Return(&TwoFactor{Secret: testTOTPSecret, Verified: true}, nil)
	store.EXPECT().MarkTwoFactorUsed(gomock.Any(), "u1", totpStep(fixedTime), fixedTime).Return(nil)
	store.EXPECT().ListOrganisationsForUser(gomock.Any(), "u1").
		Return([]Organisation{{ID: "org1", Slug: "default", Role: RoleAdmin}}, nil)
	store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(Session{ID: "s1"}, nil)

	token, err := svc.Login(testCtx, "user@example.com", "secret", "", validTOTPCode(t), "", "")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}
