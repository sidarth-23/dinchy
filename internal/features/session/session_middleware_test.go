package session_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/foundation/permission"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

var sessionFixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func newSessionService(t *testing.T, store session.Store) *session.Service {
	t.Helper()
	base := (&features.Service{Clock: clock.Fixed(sessionFixedTime), IDGenerator: id.NewGenerator()}).Named("session")
	service, err := session.NewService(base, store, config.DefaultSession(), config.DefaultCache())
	require.NoError(t, err)
	return service
}

// sessionCapture builds the Session middleware wrapping a handler that records
// whether it ran and the session it observed in context.
func sessionCapture(svc *session.Service) (http.Handler, *bool, **session.Principal) {
	ran := new(bool)
	captured := new(*session.Principal)
	handler := session.RequestMiddleware(config.DefaultSession().SessionCookieName, svc.Session)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*ran = true
		*captured = session.PrincipalFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	return handler, ran, captured
}

func validSessionRow() sqlcgen.GetSessionByTokenHashRow {
	return sqlcgen.GetSessionByTokenHashRow{
		ID:                   id.MustParse("00000000-0000-0000-0000-000000000004"),
		UserID:               id.MustParse("00000000-0000-0000-0000-000000000001"),
		Email:                "owner@example.com",
		DisplayName:          "Owner",
		ActiveOrganizationID: id.MustParse("00000000-0000-0000-0000-000000000003"),
		OrganizationName:     "Default",
		OrganizationSlug:     "default",
		Role:                 string(permission.RoleAdmin),
		IdleExpiresAt:        sqltype.Timestamptz(sessionFixedTime.Add(30 * time.Minute)),
		ExpiresAt:            sqltype.Timestamptz(sessionFixedTime.Add(7 * 24 * time.Hour)),
		RevokedAt:            pgtype.Timestamptz{},
	}
}

func TestSession_ValidCookieInjectsSession(t *testing.T) {
	t.Parallel()
	store := session.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(validSessionRow(), nil)
	svc := newSessionService(t, store)

	const token = "raw-token"
	handler, ran, captured := sessionCapture(svc)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: config.DefaultSession().SessionCookieName, Value: token})
	req = req.WithContext(support.WithRequestCookies(req.Context(), req.Cookies()))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, *ran)
	assert.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, *captured, "a valid session cookie should inject the session into context")
	assert.Equal(t, "owner@example.com", (*captured).Email)
}

func TestSession_NoCookieContinuesAnonymous(t *testing.T) {
	t.Parallel()
	// No cookie means the middleware never resolves a session, so the store is never called.
	svc := newSessionService(t, session.NewMockStore(gomock.NewController(t)))

	handler, ran, captured := sessionCapture(svc)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://example.test/", http.NoBody))

	assert.True(t, *ran, "request without a cookie should still be served")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Nil(t, *captured, "no session should be injected when the cookie is absent")
}

func TestSession_InvalidCookieContinuesAnonymous(t *testing.T) {
	t.Parallel()
	store := session.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetSessionByTokenHash(gomock.Any(), gomock.Any()).Return(sqlcgen.GetSessionByTokenHashRow{}, pgx.ErrNoRows)
	svc := newSessionService(t, store)

	handler, ran, captured := sessionCapture(svc)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: config.DefaultSession().SessionCookieName, Value: "not-a-real-token"})
	req = req.WithContext(support.WithRequestCookies(req.Context(), req.Cookies()))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, *ran, "request with an invalid cookie should still be served")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Nil(t, *captured, "an invalid session token must resolve to anonymous")
}
