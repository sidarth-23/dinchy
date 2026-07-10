package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	platformredis "github.com/sidarth-23/dinchy/internal/platform/redis"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

var (
	fixedTime                = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	testCtx                  = context.Background()
	testUserID               = "00000000-0000-0000-0000-000000000001"
	testAccountID            = "00000000-0000-0000-0000-000000000002"
	testOrganisationID       = "00000000-0000-0000-0000-000000000003"
	testSessionID            = "00000000-0000-0000-0000-000000000004"
	testOrganisationMemberID = "00000000-0000-0000-0000-000000000005"
	testVerificationTokenID  = "00000000-0000-0000-0000-000000000006"
)

func newTestService(t *testing.T) (*Service, *MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	clk := clock.Fixed(fixedTime)
	noopMailer, err := email.NewMailer(email.NoopSender{}, "")
	require.NoError(t, err)
	svc, err := NewService(nil, store, id.NewGenerator(), clk, config.DefaultAuth(), nil, newTestRedis(t), platformredis.NewKeyer("test"), noopMailer, nil)
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

func newTestRedis(t *testing.T) *goredis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
}

func HashPasswordForTest(t *testing.T, password string) string {
	t.Helper()
	hash, err := security.HashPassword(password)
	require.NoError(t, err)
	return hash
}

func findUserRow(rowID, email, displayName string) sqlcgen.FindUserByEmailRow {
	return sqlcgen.FindUserByEmailRow{ID: id.MustParse(rowID), Email: email, DisplayName: displayName, EmailVerifiedAt: sqltype.Timestamptz(fixedTime)}
}

func passwordAccountRow(rowID, userID, provider, providerAccountID, passwordHash string) sqlcgen.FindPasswordAccountByUserIDRow {
	return sqlcgen.FindPasswordAccountByUserIDRow{
		ID:                id.MustParse(rowID),
		UserID:            id.MustParse(userID),
		Provider:          provider,
		ProviderAccountID: providerAccountID,
		PasswordHash:      sqltype.Text(passwordHash),
	}
}

func organisationRow(rowID, name, slug, role string) sqlcgen.ListOrganisationsForUserRow {
	return sqlcgen.ListOrganisationsForUserRow{ID: id.MustParse(rowID), Name: name, Slug: slug, Role: role}
}

func sessionRow(rowID, userID, email, displayName, organisationID, organisationName, organisationSlug, role string, idleExpiresAt, expiresAt time.Time, revokedAt pgtype.Timestamptz) sqlcgen.GetSessionByTokenHashRow {
	return sqlcgen.GetSessionByTokenHashRow{
		ID:                   id.MustParse(rowID),
		UserID:               id.MustParse(userID),
		Email:                email,
		DisplayName:          displayName,
		ActiveOrganisationID: id.MustParse(organisationID),
		OrganisationName:     organisationName,
		OrganisationSlug:     organisationSlug,
		Role:                 role,
		IdleExpiresAt:        sqltype.Timestamptz(idleExpiresAt),
		ExpiresAt:            sqltype.Timestamptz(expiresAt),
		RevokedAt:            revokedAt,
	}
}

func TestSetupFirstUser_Success(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().
		CountUsers(gomock.Any()).
		Return(int64(0), nil)
	store.EXPECT().
		InsertUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in sqlcgen.InsertUserParams) error {
			assert.Equal(t, "admin@example.com", in.Email)
			assert.Equal(t, "Admin", in.DisplayName)
			assert.NotEqual(t, uuid.Nil, in.ID)
			assert.True(t, in.EmailVerifiedAt.Valid)
			return nil
		})
	store.EXPECT().
		InsertAccount(gomock.Any(), gomock.Any()).
		Return(nil)
	store.EXPECT().
		InsertOrganisation(gomock.Any(), gomock.Any()).
		Return(nil)
	store.EXPECT().InsertOrganisationRole(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	store.EXPECT().InsertOrganisationRolePermission(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	store.EXPECT().
		InsertOrganisationMember(gomock.Any(), gomock.Any()).
		Return(nil)
	store.EXPECT().
		InsertSession(gomock.Any(), gomock.Any()).
		Return(nil)

	token, err := svc.SetupFirstUser(testCtx, "admin@example.com", "Admin", "password123", "127.0.0.1", "ua")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestSetupFirstUser_AlreadyCompleted(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().CountUsers(gomock.Any()).Return(int64(1), nil)

	_, err := svc.SetupFirstUser(testCtx, "admin@example.com", "Admin", "pass", "", "")
	require.ErrorIs(t, err, apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 1))))
}

func TestLogin_Success(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	hashed := HashPasswordForTest(t, "secret")
	store.EXPECT().
		FindUserByEmail(gomock.Any(), "user@example.com").
		Return(findUserRow(testUserID, "user@example.com", "User"), nil)
	store.EXPECT().
		FindPasswordAccountByUserID(gomock.Any(), id.MustParse(testUserID)).
		Return(passwordAccountRow(testAccountID, testUserID, string(AccountProviderPassword), "password", hashed), nil)
	store.EXPECT().FindTwoFactorByUserID(gomock.Any(), id.MustParse(testUserID)).Return(sqlcgen.FindTwoFactorByUserIDRow{}, nil)
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(permission.RoleAdmin))}, nil)
	store.EXPECT().InsertSession(gomock.Any(), gomock.Any()).Return(nil)

	token, err := svc.Login(testCtx, "user@example.com", "secret", "", "", "127.0.0.1", "ua")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().
		FindUserByEmail(gomock.Any(), "user@example.com").
		Return(findUserRow(testUserID, "user@example.com", "User"), nil)
	store.EXPECT().
		FindPasswordAccountByUserID(gomock.Any(), id.MustParse(testUserID)).
		Return(passwordAccountRow(testAccountID, testUserID, string(AccountProviderPassword), "password", HashPasswordForTest(t, "correct")), nil)

	_, err := svc.Login(testCtx, "user@example.com", "wrong", "", "", "", "")
	require.ErrorIs(t, err, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials)))
}

func TestLogin_UserNotFound(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().FindUserByEmail(gomock.Any(), "nobody@example.com").Return(sqlcgen.FindUserByEmailRow{}, nil)

	_, err := svc.Login(testCtx, "nobody@example.com", "pass", "", "", "", "")
	require.ErrorIs(t, err, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials)))
}

func TestSession_ValidToken(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(sessionRow(testSessionID, testUserID, "user@example.com", "User", testOrganisationID, "Default", "default", string(permission.RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil)

	got, err := svc.Session(testCtx, "rawtoken")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, testSessionID, got.SessionID)
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

	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(sessionRow(testSessionID, testUserID, "user@example.com", "User", testOrganisationID, "Default", "default", string(permission.RoleAdmin), fixedTime.Add(-1*time.Second), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil)

	got, err := svc.Session(testCtx, "rawtoken")
	require.NoError(t, err)
	assert.Nil(t, got, "idle-expired session should return nil")
}

func TestSession_ExpiredAbsolute(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(sessionRow(testSessionID, testUserID, "user@example.com", "User", testOrganisationID, "Default", "default", string(permission.RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(-1*time.Second), pgtype.Timestamptz{}), nil)

	got, err := svc.Session(testCtx, "rawtoken")
	require.NoError(t, err)
	assert.Nil(t, got, "absolutely-expired session should return nil")
}

func TestSession_Revoked(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(sessionRow(testSessionID, testUserID, "user@example.com", "User", testOrganisationID, "Default", "default", string(permission.RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), sqltype.Timestamptz(fixedTime.Add(-time.Hour))), nil)

	got, err := svc.Session(testCtx, "rawtoken")
	require.NoError(t, err)
	assert.Nil(t, got, "revoked session should return nil")
}

func TestLogout_RevokesSession(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	gomock.InOrder(
		store.EXPECT().
			GetSessionByTokenHash(gomock.Any(), gomock.Any()).
			Return(sessionRow(testSessionID, testUserID, "user@example.com", "User", testOrganisationID, "Default", "default", string(permission.RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil),
		store.EXPECT().
			RevokeSessionByTokenHash(gomock.Any(), gomock.Any()).
			Return(nil),
	)

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
	hash := HashPasswordForTest(t, "mysecret")
	assert.True(t, security.VerifyPassword("mysecret", hash))
	assert.False(t, security.VerifyPassword("wrong", hash))
}

func TestPasswordHash_SamePasswordDiffers(t *testing.T) {
	t.Parallel()
	hash1 := HashPasswordForTest(t, "samePassword")
	hash2 := HashPasswordForTest(t, "samePassword")
	assert.NotEqual(t, hash1, hash2)
}
