package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

var sessionFixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

type sessionStore struct {
	session    sqlcgen.GetSessionByTokenHashRow
	sessionErr error
}

func (s *sessionStore) CountUsers(context.Context) (int64, error)                        { return 0, nil }
func (s *sessionStore) InsertUser(context.Context, sqlcgen.InsertUserParams) error       { return nil }
func (s *sessionStore) InsertAccount(context.Context, sqlcgen.InsertAccountParams) error { return nil }
func (s *sessionStore) InsertOrganisation(context.Context, sqlcgen.InsertOrganisationParams) error {
	return nil
}

func (s *sessionStore) InsertOrganisationRole(context.Context, sqlcgen.InsertOrganisationRoleParams) error {
	return nil
}

func (s *sessionStore) InsertOrganisationRolePermission(context.Context, sqlcgen.InsertOrganisationRolePermissionParams) error {
	return nil
}

func (s *sessionStore) InsertOrganisationMember(context.Context, sqlcgen.InsertOrganisationMemberParams) error {
	return nil
}

func (s *sessionStore) FindUserByEmail(context.Context, string) (sqlcgen.FindUserByEmailRow, error) {
	return sqlcgen.FindUserByEmailRow{}, nil
}

func (s *sessionStore) UpdateUserEmailVerifiedAt(context.Context, sqlcgen.UpdateUserEmailVerifiedAtParams) error {
	return nil
}

func (s *sessionStore) FindPasswordAccountByUserID(context.Context, uuid.UUID) (sqlcgen.FindPasswordAccountByUserIDRow, error) {
	return sqlcgen.FindPasswordAccountByUserIDRow{}, nil
}

func (s *sessionStore) FindUserByProviderAccount(context.Context, sqlcgen.FindUserByProviderAccountParams) (sqlcgen.FindUserByProviderAccountRow, error) {
	return sqlcgen.FindUserByProviderAccountRow{}, nil
}

func (s *sessionStore) ListOrganisationsForUser(context.Context, uuid.UUID) ([]sqlcgen.ListOrganisationsForUserRow, error) {
	return nil, nil
}

func (s *sessionStore) FindOrganisationBySlugForUser(context.Context, sqlcgen.FindOrganisationBySlugForUserParams) (sqlcgen.FindOrganisationBySlugForUserRow, error) {
	return sqlcgen.FindOrganisationBySlugForUserRow{}, nil
}

func (s *sessionStore) FindOrganisationByIDForUser(context.Context, sqlcgen.FindOrganisationByIDForUserParams) (sqlcgen.FindOrganisationByIDForUserRow, error) {
	return sqlcgen.FindOrganisationByIDForUserRow{}, nil
}

func (s *sessionStore) UpdateUserPasswordHash(context.Context, sqlcgen.UpdateUserPasswordHashParams) error {
	return nil
}

func (s *sessionStore) InsertVerificationToken(context.Context, sqlcgen.InsertVerificationTokenParams) error {
	return nil
}

func (s *sessionStore) FindVerificationToken(context.Context, sqlcgen.FindVerificationTokenParams) (sqlcgen.FindVerificationTokenRow, error) {
	return sqlcgen.FindVerificationTokenRow{}, nil
}

func (s *sessionStore) ConsumeVerificationToken(context.Context, sqlcgen.ConsumeVerificationTokenParams) error {
	return nil
}

func (s *sessionStore) InsertOrganisationInvitation(context.Context, sqlcgen.InsertOrganisationInvitationParams) error {
	return nil
}

func (s *sessionStore) FindOrganisationInvitationByToken(context.Context, string) (sqlcgen.FindOrganisationInvitationByTokenRow, error) {
	return sqlcgen.FindOrganisationInvitationByTokenRow{}, nil
}

func (s *sessionStore) FindPendingOrganisationInvitationByEmail(context.Context, sqlcgen.FindPendingOrganisationInvitationByEmailParams) (sqlcgen.FindPendingOrganisationInvitationByEmailRow, error) {
	return sqlcgen.FindPendingOrganisationInvitationByEmailRow{}, nil
}

func (s *sessionStore) ConsumeOrganisationInvitation(context.Context, sqlcgen.ConsumeOrganisationInvitationParams) error {
	return nil
}

func (s *sessionStore) InsertOrReplaceTwoFactor(context.Context, sqlcgen.InsertOrReplaceTwoFactorParams) error {
	return nil
}

func (s *sessionStore) FindTwoFactorByUserID(context.Context, uuid.UUID) (sqlcgen.FindTwoFactorByUserIDRow, error) {
	return sqlcgen.FindTwoFactorByUserIDRow{}, nil
}

func (s *sessionStore) ConfirmTwoFactor(context.Context, sqlcgen.ConfirmTwoFactorParams) error {
	return nil
}

func (s *sessionStore) MarkTwoFactorUsed(context.Context, sqlcgen.MarkTwoFactorUsedParams) error {
	return nil
}

func (s *sessionStore) RegisterTwoFactorFailure(context.Context, sqlcgen.RegisterTwoFactorFailureParams) error {
	return nil
}
func (s *sessionStore) DisableTwoFactor(context.Context, uuid.UUID) error                { return nil }
func (s *sessionStore) InsertSession(context.Context, sqlcgen.InsertSessionParams) error { return nil }
func (s *sessionStore) GetSessionByTokenHash(context.Context, string) (sqlcgen.GetSessionByTokenHashRow, error) {
	return s.session, s.sessionErr
}

func (s *sessionStore) RevokeSessionByTokenHash(context.Context, sqlcgen.RevokeSessionByTokenHashParams) error {
	return nil
}

func (s *sessionStore) RevokeSessionsForUser(context.Context, sqlcgen.RevokeSessionsForUserParams) error {
	return nil
}
func (s *sessionStore) GetInstanceName(context.Context) (string, error) { return "dinchy", nil }

func newSessionService(t *testing.T, store *sessionStore) *session.Service {
	t.Helper()
	return session.NewService(store, id.NewGenerator(), clock.Fixed(sessionFixedTime), config.DefaultSession())
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
		ActiveOrganisationID: id.MustParse("00000000-0000-0000-0000-000000000003"),
		OrganisationName:     "Default",
		OrganisationSlug:     "default",
		Role:                 string(permission.RoleAdmin),
		IdleExpiresAt:        sqltype.Timestamptz(sessionFixedTime.Add(30 * time.Minute)),
		ExpiresAt:            sqltype.Timestamptz(sessionFixedTime.Add(7 * 24 * time.Hour)),
		RevokedAt:            pgtype.Timestamptz{},
	}
}

func TestSession_ValidCookieInjectsSession(t *testing.T) {
	t.Parallel()
	store := &sessionStore{session: validSessionRow()}
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
	svc := newSessionService(t, &sessionStore{})

	handler, ran, captured := sessionCapture(svc)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://example.test/", http.NoBody))

	assert.True(t, *ran, "request without a cookie should still be served")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Nil(t, *captured, "no session should be injected when the cookie is absent")
}

func TestSession_InvalidCookieContinuesAnonymous(t *testing.T) {
	t.Parallel()
	svc := newSessionService(t, &sessionStore{sessionErr: pgx.ErrNoRows})

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
