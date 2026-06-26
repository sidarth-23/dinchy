package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
		DoAndReturn(func(_ context.Context, email string) (*User, error) {
			assert.Equal(t, "user@example.com", email)
			return &User{ID: "u1", Email: email, PasswordHash: HashPasswordForTest(t, "secret"), DisplayName: "User", Role: RoleAdmin}, nil
		})
	store.EXPECT().
		CreateSession(gomock.Any(), gomock.Any()).
		Return(Session{ID: "s1"}, nil)
	store.EXPECT().
		GetSessionByTokenHash(gomock.Any(), gomock.Any()).
		Return(&SessionWithUser{
			SessionID:     "s1",
			UserID:        "u1",
			Email:         "user@example.com",
			DisplayName:   "User",
			Role:          RoleAdmin,
			IdleExpiresAt: fixedTime.Add(30 * time.Minute),
			ExpiresAt:     fixedTime.Add(7 * 24 * time.Hour),
		}, nil)

	out, err := api.login(ctx, &LoginIn{Body: LoginBody{Email: "  USER@EXAMPLE.COM  ", Password: "secret"}})
	require.NoError(t, err)
	require.Len(t, out.SetCookie, 1)
	assert.Equal(t, "dinchy_session", out.SetCookie[0].Name)
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
		Return(&User{ID: "u1", Email: "user@example.com", PasswordHash: HashPasswordForTest(t, "correct")}, nil)

	_, err := api.login(ctx, &LoginIn{Body: LoginBody{Email: "user@example.com", Password: "wrong"}})
	require.Error(t, err)
}

func TestAPISetup_ReturnsSessionCookieAndBootstrapBody(t *testing.T) {
	t.Parallel()
	api, store := newHTTPTestAPI(t)
	ctx := testHTTPContext()

	store.EXPECT().
		CreateFirstUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in CreateUserInput) (User, error) {
			assert.Equal(t, "admin@example.com", in.Email)
			assert.Equal(t, "Admin", in.DisplayName)
			assert.NotEmpty(t, in.PasswordHash)
			return User{ID: "u1", Email: in.Email, DisplayName: in.DisplayName, Role: RoleAdmin}, nil
		})
	store.EXPECT().
		CreateSession(gomock.Any(), gomock.Any()).
		Return(Session{ID: "s1"}, nil)
	store.EXPECT().
		GetSessionByTokenHash(gomock.Any(), gomock.Any()).
		Return(&SessionWithUser{
			SessionID:     "s1",
			UserID:        "u1",
			Email:         "admin@example.com",
			DisplayName:   "Admin",
			Role:          RoleAdmin,
			IdleExpiresAt: fixedTime.Add(30 * time.Minute),
			ExpiresAt:     fixedTime.Add(7 * 24 * time.Hour),
		}, nil)

	out, err := api.setup(ctx, &SetupIn{Body: SetupBody{Email: "  ADMIN@EXAMPLE.COM  ", DisplayName: "Admin", Password: "password123"}})
	require.NoError(t, err)
	require.Len(t, out.SetCookie, 1)
	assert.Equal(t, "dinchy_session", out.SetCookie[0].Name)
	assert.True(t, out.Body.Authenticated)
	require.NotNil(t, out.Body.Viewer)
	assert.Equal(t, "admin@example.com", out.Body.Viewer.Email)
}

func TestAPISession_ReturnsCurrentViewer(t *testing.T) {
	t.Parallel()
	api, _ := newHTTPTestAPI(t)
	ctx := WithSession(testCtx, &SessionWithUser{
		SessionID:     "s1",
		UserID:        "u1",
		Email:         "viewer@example.com",
		DisplayName:   "Viewer",
		Role:          RoleAdmin,
		IdleExpiresAt: fixedTime.Add(30 * time.Minute),
		ExpiresAt:     fixedTime.Add(7 * 24 * time.Hour),
	})

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
	api, _ := newHTTPTestAPI(t)
	ctx := WithSession(context.Background(), &SessionWithUser{
		SessionID:   "s1",
		UserID:      "u1",
		Email:       "viewer@example.com",
		DisplayName: "Viewer",
		Role:        RoleAdmin,
	})

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

	store.EXPECT().RevokeSessionByTokenHash(gomock.Any(), hashToken("rawtoken")).Return(nil)

	out, err := api.logout(testCtx, &LogoutIn{DinchySession: "rawtoken"})
	require.NoError(t, err)
	assert.Equal(t, "dinchy_session", out.SetCookie.Name)
	assert.Equal(t, -1, out.SetCookie.MaxAge)
}
