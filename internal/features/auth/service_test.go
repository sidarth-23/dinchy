package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/sidarth-23/dinchy/internal/features/auth/errs"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/platform/id"
)

var (
	fixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	testCtx   = context.Background()
)

func newTestService(t *testing.T) (*Service, *MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	clk := fakeClock{now: fixedTime}
	svc := NewService(store, id.NewGenerator(), clk)
	return svc, store
}

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time {
	return c.now
}

func TestSetupFirstUser_Success(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().
		CreateFirstUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in CreateUserInput) (User, error) {
			assert.Equal(t, "admin@example.com", in.Email)
			assert.Equal(t, "Admin", in.DisplayName)
			assert.NotEmpty(t, in.PasswordHash)
			return User{ID: "user-1", Email: in.Email, DisplayName: in.DisplayName, Role: RoleAdmin}, nil
		})
	store.EXPECT().
		CreateSession(gomock.Any(), gomock.Any()).
		Return(session.Session{ID: "sess-1"}, nil)

	token, err := svc.SetupFirstUser(testCtx, "admin@example.com", "Admin", "password123", "127.0.0.1", "ua")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestSetupFirstUser_EmailNormalized(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().
		CreateFirstUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in CreateUserInput) (User, error) {
			assert.Equal(t, "admin@example.com", in.Email, "email must be normalized to lowercase")
			return User{ID: "user-1", Email: in.Email, Role: RoleAdmin}, nil
		})
	store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(session.Session{ID: "sess-1"}, nil)

	_, err := svc.SetupFirstUser(testCtx, "  ADMIN@EXAMPLE.COM  ", "Admin", "password123", "", "")
	require.NoError(t, err)
}

func TestSetupFirstUser_AlreadyCompleted(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().CreateFirstUser(gomock.Any(), gomock.Any()).Return(User{}, errs.ErrSetupCompleted)

	_, err := svc.SetupFirstUser(testCtx, "admin@example.com", "Admin", "pass", "", "")
	require.ErrorIs(t, err, errs.ErrSetupCompleted)
}

func TestLogin_Success(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	hashed := hashPassword("secret")
	store.EXPECT().
		FindUserByEmail(gomock.Any(), "user@example.com").
		Return(&User{ID: "u1", Email: "user@example.com", PasswordHash: hashed, Role: RoleAdmin}, nil)
	store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(session.Session{ID: "s1"}, nil)

	token, err := svc.Login(testCtx, "user@example.com", "secret", "127.0.0.1", "ua")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().
		FindUserByEmail(gomock.Any(), "user@example.com").
		Return(&User{ID: "u1", Email: "user@example.com", PasswordHash: hashPassword("correct")}, nil)

	_, err := svc.Login(testCtx, "user@example.com", "wrong", "", "")
	require.ErrorIs(t, err, errs.ErrInvalidCredentials)
}

func TestLogin_UserNotFound(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().FindUserByEmail(gomock.Any(), "nobody@example.com").Return(nil, nil)

	_, err := svc.Login(testCtx, "nobody@example.com", "pass", "", "")
	require.ErrorIs(t, err, errs.ErrInvalidCredentials)
}

func TestLogin_EmailNormalized(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().
		FindUserByEmail(gomock.Any(), "user@example.com").
		Return(&User{ID: "u1", PasswordHash: hashPassword("p")}, nil)
	store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(session.Session{ID: "s1"}, nil)

	_, err := svc.Login(testCtx, "  USER@EXAMPLE.COM  ", "p", "", "")
	require.NoError(t, err)
}

func TestSession_ValidToken(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	sess := &session.SessionWithUser{
		SessionID:     "s1",
		UserID:        "u1",
		IdleExpiresAt: fixedTime.Add(30 * time.Minute),
		ExpiresAt:     fixedTime.Add(7 * 24 * time.Hour),
	}
	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(sess, nil)

	got, err := svc.Session(testCtx, "rawtoken")
	require.NoError(t, err)
	assert.Equal(t, sess, got)
}

func TestSession_EmptyToken(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)

	got, err := svc.Session(testCtx, "")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSession_ExpiredIdle(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	sess := &session.SessionWithUser{
		IdleExpiresAt: fixedTime.Add(-1 * time.Second), // expired
		ExpiresAt:     fixedTime.Add(7 * 24 * time.Hour),
	}
	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(sess, nil)

	got, err := svc.Session(testCtx, "rawtoken")
	require.NoError(t, err)
	assert.Nil(t, got, "idle-expired session should return nil")
}

func TestSession_ExpiredAbsolute(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	sess := &session.SessionWithUser{
		IdleExpiresAt: fixedTime.Add(30 * time.Minute),
		ExpiresAt:     fixedTime.Add(-1 * time.Second), // expired
	}
	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(sess, nil)

	got, err := svc.Session(testCtx, "rawtoken")
	require.NoError(t, err)
	assert.Nil(t, got, "absolutely-expired session should return nil")
}

func TestSession_Revoked(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	sess := &session.SessionWithUser{
		IdleExpiresAt: fixedTime.Add(30 * time.Minute),
		ExpiresAt:     fixedTime.Add(7 * 24 * time.Hour),
		RevokedAt:     sql.NullTime{Time: fixedTime.Add(-time.Hour), Valid: true},
	}
	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(sess, nil)

	got, err := svc.Session(testCtx, "rawtoken")
	require.NoError(t, err)
	assert.Nil(t, got, "revoked session should return nil")
}

func TestLogout_RevokesSession(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().RevokeSessionByTokenHash(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, svc.Logout(testCtx, "rawtoken"))
}

func TestLogout_EmptyToken(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	// No store calls expected.
	require.NoError(t, svc.Logout(testCtx, ""))
}

func TestPasswordHash_RoundTrip(t *testing.T) {
	t.Parallel()
	hash := hashPassword("mysecret")
	assert.True(t, verifyPassword("mysecret", hash))
	assert.False(t, verifyPassword("wrong", hash))
}
