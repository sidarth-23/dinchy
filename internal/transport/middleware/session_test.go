package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cachecore "github.com/sidarth-23/dinchy/internal/cache/core"
	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/store/testsupport"
	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/transport/middleware"
)

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time { return c.now }

var sessionFixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func newSessionService(t *testing.T) *auth.Service {
	t.Helper()
	db := testsupport.OpenPostgresStore(t)
	queries := sqlcgen.New(db.DB())
	svc, err := auth.NewService(
		db.DB(),
		queries,
		id.NewGenerator(),
		fakeClock{now: sessionFixedTime},
		config.DefaultAuth(),
		nil,
		nil,
		cachecore.NewKeyer("test"),
		email.NoopSender{},
	)
	require.NoError(t, err)
	return svc
}

// sessionCapture builds the Session middleware wrapping a handler that records
// whether it ran and the session it observed in context.
func sessionCapture(svc *auth.Service) (http.Handler, *bool, **auth.SessionWithUser) {
	ran := new(bool)
	captured := new(*auth.SessionWithUser)
	handler := middleware.Session(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*ran = true
		*captured = auth.SessionFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	return handler, ran, captured
}

func TestSession_ValidCookieInjectsSession(t *testing.T) {
	t.Parallel()
	svc := newSessionService(t)

	token, err := svc.SetupFirstUser(context.Background(), "owner@example.com", "Owner", "password123", "127.0.0.1", "test-agent")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	handler, ran, captured := sessionCapture(svc)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.AddCookie(&http.Cookie{Name: svc.SessionCookieName(), Value: token})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, *ran)
	assert.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, *captured, "a valid session cookie should inject the session into context")
	assert.Equal(t, "owner@example.com", (*captured).Email)
}

func TestSession_NoCookieContinuesAnonymous(t *testing.T) {
	t.Parallel()
	svc := newSessionService(t)

	handler, ran, captured := sessionCapture(svc)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://example.test/", nil))

	assert.True(t, *ran, "request without a cookie should still be served")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Nil(t, *captured, "no session should be injected when the cookie is absent")
}

func TestSession_InvalidCookieContinuesAnonymous(t *testing.T) {
	t.Parallel()
	svc := newSessionService(t)

	handler, ran, captured := sessionCapture(svc)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.AddCookie(&http.Cookie{Name: svc.SessionCookieName(), Value: "not-a-real-token"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, *ran, "request with an invalid cookie should still be served")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Nil(t, *captured, "an invalid session token must resolve to anonymous")
}
