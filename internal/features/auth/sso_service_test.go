package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"

	cachecore "github.com/sidarth-23/dinchy/internal/cache/core"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

type fakeSSOSession struct {
	AuthURL     string `json:"auth_url"`
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
}

func (s *fakeSSOSession) GetAuthURL() (string, error) {
	return s.AuthURL, nil
}

func (s *fakeSSOSession) Marshal() string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

func (s *fakeSSOSession) Authorize(_ goth.Provider, params goth.Params) (string, error) {
	s.AccessToken = params.Get("code")
	if s.AccessToken == "" {
		s.AccessToken = "access-token"
	}
	return s.AccessToken, nil
}

type fakeSSOProvider struct {
	name string
}

func (p *fakeSSOProvider) Name() string {
	return p.name
}

func (p *fakeSSOProvider) SetName(name string) {
	p.name = name
}

func (p *fakeSSOProvider) BeginAuth(state string) (goth.Session, error) {
	return &fakeSSOSession{
		AuthURL: fmt.Sprintf("https://sso.example.test/auth?state=%s", url.QueryEscape(state)),
		UserID:  "provider-user",
		Email:   "candidate@example.com",
	}, nil
}

func (p *fakeSSOProvider) UnmarshalSession(raw string) (goth.Session, error) {
	if raw == "" {
		return nil, errors.New("missing session")
	}
	var session fakeSSOSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, err
	}
	if session.UserID == "" {
		session.UserID = "provider-user"
	}
	if session.Email == "" {
		session.Email = "candidate@example.com"
	}
	return &session, nil
}

func (p *fakeSSOProvider) FetchUser(session goth.Session) (goth.User, error) {
	fakeSession, ok := session.(*fakeSSOSession)
	if !ok {
		return goth.User{}, errors.New("unexpected session type")
	}
	if fakeSession.AccessToken == "" {
		return goth.User{}, errors.New("missing access token")
	}
	return goth.User{
		Provider:    p.name,
		UserID:      fakeSession.UserID,
		Email:       fakeSession.Email,
		AccessToken: fakeSession.AccessToken,
	}, nil
}

func (p *fakeSSOProvider) Debug(bool) {}

func (p *fakeSSOProvider) RefreshToken(string) (*oauth2.Token, error) {
	return nil, errors.New("not implemented")
}

func (p *fakeSSOProvider) RefreshTokenAvailable() bool {
	return false
}

func newSSOTestService(t *testing.T) (*Service, *MockStore) {
	t.Helper()
	svc, store := newTestService(t)
	svc.sso = &ssoRegistry{
		stateCookieName: "dinchy_sso_state",
		stateLifetime:   time.Minute,
		envProviders: map[string]config.SSOProviderConfig{
			"github": {
				ID:          config.SSOProviderGitHub,
				Name:        "GitHub",
				ClientID:    "client-id",
				Secret:      "secret",
				CallbackURL: "https://app.example.test/api/auth/sso/github/callback",
				Enabled:     true,
			},
		},
		cacheKeyer: cachecore.NewKeyer("test"),
	}
	originalProviderFactory := newGothProviderForSSO
	newGothProviderForSSO = func(cfg config.SSOProviderConfig) (goth.Provider, error) {
		return &fakeSSOProvider{name: string(cfg.ID)}, nil
	}
	t.Cleanup(func() { newGothProviderForSSO = originalProviderFactory })
	store.EXPECT().ListSSOProviderSettings(gomock.Any()).Return(nil, nil).AnyTimes()
	return svc, store
}

func cookieValue(t *testing.T, cookies []http.Cookie, name string) string {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	t.Fatalf("cookie %q not found", name)
	return ""
}

func TestStartSSO_ReturnsMetadataAndTransactionCookie(t *testing.T) {
	svc, _ := newSSOTestService(t)

	authURL, cookies, err := svc.startSSO(testCtx, "github", "/projects/123?tab=activity", "default")
	require.NoError(t, err)
	require.Len(t, cookies, 1)
	assert.Contains(t, authURL, "state=")

	transactionID := cookieValue(t, cookies, "dinchy_sso_state")
	var cached ssoCacheState
	require.NoError(t, cachecore.GetJSON(testCtx, svc.cache, svc.sso.cacheKey(transactionID), &cached))
	assert.Equal(t, "github", cached.ProviderID)
	assert.Equal(t, "/projects/123?tab=activity", cached.ReturnTo)
	assert.Equal(t, "default", cached.OrganisationSlug)
	var session fakeSSOSession
	require.NoError(t, json.Unmarshal([]byte(cached.Session), &session))
	assert.Contains(t, session.AuthURL, "state=")
}

func TestCompleteSSO_FallsBackToEmailAndClearsCookies(t *testing.T) {
	svc, store := newSSOTestService(t)

	_, cookies, err := svc.startSSO(testCtx, "github", "/dashboard", "")
	require.NoError(t, err)

	transactionID := cookieValue(t, cookies, "dinchy_sso_state")
	var cached ssoCacheState
	require.NoError(t, cachecore.GetJSON(testCtx, svc.cache, svc.sso.cacheKey(transactionID), &cached))
	var session fakeSSOSession
	require.NoError(t, json.Unmarshal([]byte(cached.Session), &session))

	parsedAuthURL, err := url.Parse(session.AuthURL)
	require.NoError(t, err)
	state := parsedAuthURL.Query().Get("state")

	store.EXPECT().
		FindUserByProviderAccount(gomock.Any(), "github", "provider-user").
		Return(nil, nil)
	store.EXPECT().
		FindUserByEmail(gomock.Any(), "candidate@example.com").
		Return(&User{ID: "user-1", Email: "candidate@example.com"}, nil)
	store.EXPECT().
		ListOrganisationsForUser(gomock.Any(), "user-1").
		Return([]Organisation{{ID: "org-1", Name: "Default", Slug: "default", Role: RoleAdmin}}, nil)
	store.EXPECT().
		CreateSession(gomock.Any(), gomock.Any()).
		Return(Session{ID: "sess-1"}, nil)

	returnTo, token, clearedCookies, err := svc.completeSSO(
		testCtx,
		"github",
		state,
		"code-123",
		cookieValue(t, cookies, "dinchy_sso_state"),
		"127.0.0.1",
		"ua",
	)
	require.NoError(t, err)
	assert.Equal(t, "/dashboard", returnTo)
	assert.NotEmpty(t, token)
	require.Len(t, clearedCookies, 1)
	assert.Equal(t, -1, clearedCookies[0].MaxAge)
}

func TestCompleteSSO_RejectsInvalidState(t *testing.T) {
	svc, _ := newSSOTestService(t)

	_, cookies, err := svc.startSSO(testCtx, "github", "/dashboard", "default")
	require.NoError(t, err)

	_, _, _, err = svc.completeSSO(
		testCtx,
		"github",
		"wrong-state",
		"code-123",
		cookieValue(t, cookies, "dinchy_sso_state"),
		"",
		"",
	)
	require.ErrorIs(t, err, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState)))
}
