package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/events"
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
		sessions:     svc.sessions,
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
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(permission.RoleAdmin))}, nil).
		AnyTimes()
	store.EXPECT().InsertSession(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().
		GetSessionByTokenHash(gomock.Any(), gomock.Any()).
		Return(sessionRow(testSessionID, testUserID, "user@example.com", "User", testOrganisationID, "Default", "default", string(permission.RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil)

	out, err := api.login(ctx, &LoginIn{Body: LoginBody{Email: "user@example.com", Password: "secret"}})
	require.NoError(t, err)
	require.Len(t, out.SetCookie, 1)
	assert.Equal(t, api.sessions.SessionCookieName(), out.SetCookie[0].Name)
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
	store.EXPECT().InsertOrganisationRole(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	store.EXPECT().InsertOrganisationRolePermission(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	store.EXPECT().InsertOrganisationMember(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().InsertSession(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().
		GetSessionByTokenHash(gomock.Any(), gomock.Any()).
		Return(sessionRow(testSessionID, testUserID, "admin@example.com", "Admin", testOrganisationID, "Default", "default", string(permission.RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil)
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(permission.RoleAdmin))}, nil).
		AnyTimes()

	out, err := api.setup(ctx, &SetupIn{Body: SetupBody{Email: "admin@example.com", DisplayName: "Admin", Password: "password123"}})
	require.NoError(t, err)
	require.Len(t, out.SetCookie, 1)
	assert.Equal(t, api.sessions.SessionCookieName(), out.SetCookie[0].Name)
	assert.True(t, out.Body.Authenticated)
	require.NotNil(t, out.Body.Viewer)
	assert.Equal(t, "admin@example.com", out.Body.Viewer.Email)
}

func TestAPISession_ReturnsCurrentViewer(t *testing.T) {
	t.Parallel()
	api, store := newHTTPTestAPI(t)
	ctx := session.WithPrincipal(testCtx, &session.Principal{
		SessionID:        testSessionID,
		UserID:           testUserID,
		Email:            "viewer@example.com",
		DisplayName:      "Viewer",
		OrganisationID:   testOrganisationID,
		OrganisationName: "Default",
		OrganisationSlug: "default",
		Role:             permission.RoleAdmin,
		IdleExpiresAt:    fixedTime.Add(30 * time.Minute),
		ExpiresAt:        fixedTime.Add(7 * 24 * time.Hour),
	})
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(permission.RoleAdmin))}, nil).
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
	ctx := session.WithPrincipal(context.Background(), &session.Principal{
		SessionID:        testSessionID,
		UserID:           testUserID,
		Email:            "viewer@example.com",
		DisplayName:      "Viewer",
		OrganisationID:   testOrganisationID,
		OrganisationName: "Default",
		OrganisationSlug: "default",
		Role:             permission.RoleAdmin,
	})
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(permission.RoleAdmin))}, nil).
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
	publisher := &recordingPublisher{}
	api.auth.publisher = publisher
	ctx := support.WithRequestCookies(testCtx, []*http.Cookie{{Name: api.sessions.SessionCookieName(), Value: "rawtoken"}})

	gomock.InOrder(
		store.EXPECT().
			GetSessionByTokenHash(gomock.Any(), gomock.Any()).
			Return(sessionRow(testSessionID, testUserID, "user@example.com", "User", testOrganisationID, "Default", "default", string(permission.RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil),
		store.EXPECT().RevokeSessionByTokenHash(gomock.Any(), sqlcgen.RevokeSessionByTokenHashParams{RevokedAt: sqltype.Timestamptz(fixedTime), UpdatedAt: sqltype.Timestamptz(fixedTime), TokenHash: security.HashToken("rawtoken")}).
			Return(nil),
	)

	out, err := api.logout(ctx, &LogoutIn{})
	require.NoError(t, err)
	assert.Equal(t, api.sessions.SessionCookieName(), out.SetCookie.Name)
	assert.Equal(t, -1, out.SetCookie.MaxAge)
	require.NotNil(t, publisher.event)
	require.Equal(t, events.AuthSecurityAuthLogoutSucceeded, publisher.event.Type())
	envelope := publisher.event.EnvelopeData()
	assert.Equal(t, "session", envelope.TargetType)
	assert.Equal(t, testSessionID, envelope.TargetID)
	assert.Equal(t, "User", envelope.TargetDisplay)
}

func TestAPISSOStart_SetsSecureOnAllCookies(t *testing.T) {
	svc, _ := newSSOTestService(t)
	api := &API{auth: svc, sessions: svc.sessions, settings: testSettingsReader{state: BootstrapState{InstanceName: "dinchy"}}, requireHTTPS: false}

	out, err := api.ssoStart(support.WithSecure(context.Background(), true), &SSOStartIn{ProviderID: "github", ReturnTo: "/dashboard"})
	require.NoError(t, err)
	require.Len(t, out.SetCookie, 1)
	assert.True(t, out.SetCookie[0].Secure)
}

func TestAPISSOCallback_SetsSecureOnSessionAndClearCookies(t *testing.T) {
	api, store := newHTTPTestAPI(t)
	svc, _ := newSSOTestService(t)
	api.auth.sso = svc.sso

	_, cookies, err := api.auth.startSSO(testCtx, "github", "/dashboard", "")
	require.NoError(t, err)
	transactionID := cookieValue(t, cookies, "dinchy_sso_state")
	var cached ssoCacheState
	require.NoError(t, api.auth.redis.Get(testCtx, api.auth.sso.cacheKey(transactionID)).Scan(&cached))
	var session fakeSSOSession
	require.NoError(t, json.Unmarshal([]byte(cached.ProviderSession), &session))
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
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(permission.RoleAdmin))}, nil).
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

// The following tests exercise the full huma pipeline (schema validation and
// resolvers run before the handler) via humatest, confirming registration does
// not panic on the operation tags and that strict validation and boundary
// normalization behave as intended.

func TestHumaValidation_RejectsInvalidInvitationRole(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	_, api := humatest.New(t)
	Register(api, svc, svc.sessions, testSettingsReader{state: BootstrapState{InstanceName: "dinchy"}}, false)

	// "owner" is not in the role enum; validation rejects it before the handler runs.
	resp := api.Post("/auth/invitations", map[string]any{"email": "invitee@example.com", "role": "owner"})
	require.Equal(t, http.StatusUnauthorized, resp.Code, resp.Body.String())
}

func TestHumaValidation_RejectsMalformedEmail(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	_, api := humatest.New(t)
	Register(api, svc, svc.sessions, testSettingsReader{state: BootstrapState{InstanceName: "dinchy"}}, false)

	// A malformed address fails format:email validation before the handler runs.
	resp := api.Post("/auth/forgot-password", map[string]any{"email": "not-an-email"})
	require.Equal(t, http.StatusUnprocessableEntity, resp.Code, resp.Body.String())
}

func TestHumaResolver_LowercasesSetupEmailEndToEnd(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)
	store.EXPECT().CountUsers(gomock.Any()).Return(int64(0), nil)
	store.EXPECT().
		InsertUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in sqlcgen.InsertUserParams) error {
			assert.Equal(t, "admin@example.com", in.Email, "resolver must lowercase the email before the handler")
			return nil
		})
	store.EXPECT().InsertAccount(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().InsertOrganisation(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().InsertOrganisationRole(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	store.EXPECT().InsertOrganisationRolePermission(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	store.EXPECT().InsertOrganisationMember(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().InsertSession(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().
		GetSessionByTokenHash(gomock.Any(), gomock.Any()).
		Return(sessionRow(testSessionID, testUserID, "admin@example.com", "Admin", testOrganisationID, "Default", "default", string(permission.RoleAdmin), fixedTime.Add(30*time.Minute), fixedTime.Add(7*24*time.Hour), pgtype.Timestamptz{}), nil)
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), id.MustParse(testUserID)).
		Return([]sqlcgen.ListOrganisationsForUserRow{organisationRow(testOrganisationID, "Default", "default", string(permission.RoleAdmin))}, nil).
		AnyTimes()

	_, api := humatest.New(t)
	Register(api, svc, svc.sessions, testSettingsReader{state: BootstrapState{InstanceName: "dinchy"}}, false)

	resp := api.Post("/setup/first-user", map[string]any{
		"email":        "ADMIN@EXAMPLE.COM",
		"display_name": "Admin",
		"password":     "password123",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
}
