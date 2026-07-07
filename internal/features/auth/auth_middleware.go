package auth

import (
	"context"
	"net/http"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// WithSession returns a context carrying the given session for downstream handlers.
func WithSession(ctx context.Context, s *SessionWithUser) context.Context {
	return context.WithValue(ctx, ctxKeySession, s)
}

// SessionFrom returns the session stored in the context, or nil when none is present.
func SessionFrom(ctx context.Context) *SessionWithUser {
	s, _ := ctx.Value(ctxKeySession).(*SessionWithUser)
	return s
}

// SessionCookieName returns the configured name of the session cookie.
func (s *Service) SessionCookieName() string { return s.authConfig.SessionCookieName }

// SSOStateCookieName returns the configured name of the SSO state cookie.
func (s *Service) SSOStateCookieName() string { return s.authConfig.SSOStateCookieName }

// SessionCookie builds a session cookie carrying the given token, marking it secure when requested.
func (s *Service) SessionCookie(token string, secure bool) *http.Cookie {
	return support.ValueCookie(s.authConfig.SessionCookieName, token, secure)
}

// ClearSessionCookie builds a cookie that clears the session cookie on the client.
func (s *Service) ClearSessionCookie(secure bool) *http.Cookie {
	return support.ClearCookie(s.authConfig.SessionCookieName, secure)
}
