package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cachecore "github.com/sidarth-23/dinchy/internal/platform/cache/core"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

type testSettingsReader struct {
	state BootstrapState
}

func (r testSettingsReader) Bootstrap(context.Context) (BootstrapState, error) {
	return r.state, nil
}

func newHTTPTestAPI(t *testing.T) (*API, *MockStore) {
	t.Helper()
	svc, store := newTestService(t)
	api := &API{
		auth:         svc,
		settings:     testSettingsReader{state: BootstrapState{InstanceName: "dinchy"}},
		requireHTTPS: false,
	}
	return api, store
}

func testHTTPContext() context.Context {
	return support.WithRequestInfo(testCtx, "127.0.0.1", "ua")
}

func TestAPILogin_Success(t *testing.T) {
	t.Parallel()
	api, store := newHTTPTestAPI(t)
	ctx := testHTTPContext()

	store.EXPECT().
		FindUserByEmail(gomock.Any(), "user@example.com").
		Return(findUserRow(testUserID, "user@example.com", "User"), nil)
	store.EXPECT().
		FindPasswordAccountByUserID(gomock.Any(), id.MustParse(testUserID)).
		Return(passwordAccountRow(testAccountID, testUserID, string(AccountProviderPassword), "password", HashPasswordForTest(t, "secret")), nil)
	store.EXPECT().FindTwoFactorByUserID(gomock.Any(), id.MustParse(testUserID)).Return(sqlcgen.FindTwoFactorByUserIDRow{}, nil)
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(RoleAdmin))}, nil).
		AnyTimes()
	store.EXPECT().InsertSession(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().
		GetSessionByTokenHash(gomock.Any(), gomock.Any()).
		Return(sessionRow(testSessionID, testUserID, "user@example.com", "User", testOrganisationID, "Default", "default", string(RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil)

	out, err := api.login(ctx, &LoginIn{Body: LoginBody{Email: "  USER@EXAMPLE.COM  ", Password: "secret"}})
	require.NoError(t, err)
	require.Len(t, out.SetCookie, 1)
	assert.Equal(t, api.auth.SessionCookieName(), out.SetCookie[0].Name)
	assert.False(t, out.SetCookie[0].Secure)
	assert.True(t, out.Body.Authenticated)
	assert.Equal(t, "dinchy", out.Body.App.InstanceName)
	require.NotNil(t, out.Body.Viewer)
	assert.Equal(t, "user@example.com", out.Body.Viewer.Email)
}

func TestAPILogin_WrongPassword(t *testing.T) {
	t.Parallel()
	api, store := newHTTPTestAPI(t)
	ctx := testHTTPContext()

	store.EXPECT().
		FindUserByEmail(gomock.Any(), "user@example.com").
		Return(findUserRow(testUserID, "user@example.com", "User"), nil)
	store.EXPECT().
		FindPasswordAccountByUserID(gomock.Any(), id.MustParse(testUserID)).
		Return(passwordAccountRow(testAccountID, testUserID, string(AccountProviderPassword), "password", HashPasswordForTest(t, "correct")), nil)

	_, err := api.login(ctx, &LoginIn{Body: LoginBody{Email: "user@example.com", Password: "wrong"}})
	require.Error(t, err)
}

func TestAPISetup_ReturnsSessionCookieAndBootstrapBody(t *testing.T) {
	t.Parallel()
	api, store := newHTTPTestAPI(t)
	ctx := testHTTPContext()

	store.EXPECT().
		CountUsers(gomock.Any()).
		Return(int64(0), nil)
	store.EXPECT().
		InsertUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in sqlcgen.InsertUserParams) error {
			assert.Equal(t, "admin@example.com", in.Email)
			assert.Equal(t, "Admin", in.DisplayName)
			assert.NotEqual(t, uuid.Nil, in.ID)
			return nil
		})
	store.EXPECT().InsertAccount(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().InsertOrganisation(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().InsertOrganisationMember(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().InsertSession(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().
		GetSessionByTokenHash(gomock.Any(), gomock.Any()).
		Return(sessionRow(testSessionID, testUserID, "admin@example.com", "Admin", testOrganisationID, "Default", "default", string(RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil)
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(RoleAdmin))}, nil).
		AnyTimes()

	out, err := api.setup(ctx, &SetupIn{Body: SetupBody{Email: "  ADMIN@EXAMPLE.COM  ", DisplayName: "Admin", Password: "password123"}})
	require.NoError(t, err)
	require.Len(t, out.SetCookie, 1)
	assert.Equal(t, api.auth.SessionCookieName(), out.SetCookie[0].Name)
	assert.True(t, out.Body.Authenticated)
	require.NotNil(t, out.Body.Viewer)
	assert.Equal(t, "admin@example.com", out.Body.Viewer.Email)
}

func TestAPISession_ReturnsCurrentViewer(t *testing.T) {
	t.Parallel()
	api, store := newHTTPTestAPI(t)
	ctx := WithSession(testCtx, &SessionWithUser{
		SessionID:        testSessionID,
		UserID:           testUserID,
		Email:            "viewer@example.com",
		DisplayName:      "Viewer",
		OrganisationID:   testOrganisationID,
		OrganisationName: "Default",
		OrganisationSlug: "default",
		Role:             RoleAdmin,
		IdleExpiresAt:    fixedTime.Add(30 * time.Minute),
		ExpiresAt:        fixedTime.Add(7 * 24 * time.Hour),
	})
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(RoleAdmin))}, nil).
		AnyTimes()

	out, err := api.session(ctx, &struct{}{})
	require.NoError(t, err)
	assert.True(t, out.Body.Authenticated)
	require.NotNil(t, out.Body.Viewer)
	assert.Equal(t, "viewer@example.com", out.Body.Viewer.Email)
}

func TestAPIBootstrap_Anonymous(t *testing.T) {
	t.Parallel()
	api, _ := newHTTPTestAPI(t)
	api.settings = testSettingsReader{state: BootstrapState{SetupRequired: true, InstanceName: "dinchy"}}

	out, err := api.bootstrap(context.Background(), &struct{}{})
	require.NoError(t, err)
	assert.True(t, out.Body.SetupRequired)
	assert.False(t, out.Body.Authenticated)
	assert.Equal(t, "dinchy", out.Body.App.InstanceName)
}

func TestAPIBootstrap_WithSession(t *testing.T) {
	t.Parallel()
	api, store := newHTTPTestAPI(t)
	ctx := WithSession(context.Background(), &SessionWithUser{
		SessionID:        testSessionID,
		UserID:           testUserID,
		Email:            "viewer@example.com",
		DisplayName:      "Viewer",
		OrganisationID:   testOrganisationID,
		OrganisationName: "Default",
		OrganisationSlug: "default",
		Role:             RoleAdmin,
	})
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(RoleAdmin))}, nil).
		AnyTimes()

	out, err := api.bootstrap(ctx, &struct{}{})
	require.NoError(t, err)
	assert.True(t, out.Body.Authenticated)
	require.NotNil(t, out.Body.Viewer)
	assert.Equal(t, "viewer@example.com", out.Body.Viewer.Email)
	assert.Equal(t, "Viewer", out.Body.Viewer.DisplayName)
}

func TestAPILogout_ClearsCookie(t *testing.T) {
	t.Parallel()
	api, store := newHTTPTestAPI(t)
	ctx := support.WithRequestCookies(testCtx, []*http.Cookie{{Name: api.auth.SessionCookieName(), Value: "rawtoken"}})

	store.EXPECT().
		GetSessionByTokenHash(gomock.Any(), gomock.Any()).
		Return(sessionRow(testSessionID, testUserID, "user@example.com", "User", testOrganisationID, "Default", "default", string(RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil)
	store.EXPECT().RevokeSessionByTokenHash(gomock.Any(), sqlcgen.RevokeSessionByTokenHashParams{RevokedAt: sqltype.Timestamptz(fixedTime), UpdatedAt: sqltype.Timestamptz(fixedTime), TokenHash: security.HashToken("rawtoken")}).Return(nil)

	out, err := api.logout(ctx, &LogoutIn{})
	require.NoError(t, err)
	assert.Equal(t, api.auth.SessionCookieName(), out.SetCookie.Name)
	assert.Equal(t, -1, out.SetCookie.MaxAge)
}

func TestAPISSOStart_SetsSecureOnAllCookies(t *testing.T) {
	svc, _ := newSSOTestService(t)
	api := &API{auth: svc, settings: testSettingsReader{state: BootstrapState{InstanceName: "dinchy"}}, requireHTTPS: false}

	out, err := api.ssoStart(support.WithSecure(context.Background(), true), &SSOStartIn{ProviderID: "github", ReturnTo: "/dashboard"})
	require.NoError(t, err)
	require.Len(t, out.SetCookie, 1)
	assert.True(t, out.SetCookie[0].Secure)
}

func TestAPISSOCallback_SetsSecureOnSessionAndClearCookies(t *testing.T) {
	api, store := newHTTPTestAPI(t)
	svc, _ := newSSOTestService(t)
	api.auth.sso = svc.sso
	store.EXPECT().ListSSOProviderSettings(gomock.Any()).Return(nil, nil).AnyTimes()

	_, cookies, err := api.auth.startSSO(testCtx, "github", "/dashboard", "")
	require.NoError(t, err)
	transactionID := cookieValue(t, cookies, "dinchy_sso_state")
	var cached ssoCacheState
	require.NoError(t, cachecore.GetJSON(testCtx, api.auth.cache, api.auth.sso.cacheKey(transactionID), &cached))
	var session fakeSSOSession
	require.NoError(t, json.Unmarshal([]byte(cached.Session), &session))
	parsedAuthURL, err := url.Parse(session.AuthURL)
	require.NoError(t, err)

	store.EXPECT().
		FindUserByProviderAccount(gomock.Any(), sqlcgen.FindUserByProviderAccountParams{Provider: "github", ProviderAccountID: "provider-user"}).
		Return(sqlcgen.FindUserByProviderAccountRow{}, pgx.ErrNoRows)
	store.EXPECT().
		FindUserByEmail(gomock.Any(), "candidate@example.com").
		Return(findUserRow(testUserID, "candidate@example.com", "User"), nil)
	store.EXPECT().InsertAccount(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(RoleAdmin))}, nil).
		AnyTimes()
	store.EXPECT().InsertSession(gomock.Any(), gomock.Any()).Return(nil)

	requestCookies := make([]*http.Cookie, 0, len(cookies))
	for i := range cookies {
		requestCookies = append(requestCookies, &cookies[i])
	}
	ctx := support.WithSecure(
		support.WithRequestInfo(
			support.WithRequestCookies(context.Background(), requestCookies),
			"127.0.0.1",
			"ua",
		),
		true,
	)
	out, err := api.ssoCallback(ctx, &SSOCallbackIn{ProviderID: "github", Code: "code-123", State: parsedAuthURL.Query().Get("state")})
	require.NoError(t, err)
	require.Len(t, out.SetCookie, 2)
	assert.True(t, out.SetCookie[0].Secure)
	assert.True(t, out.SetCookie[1].Secure)
}
