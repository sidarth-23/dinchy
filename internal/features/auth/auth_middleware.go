package auth

import (
	"context"
	"net/http"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

func (s *Service) SessionCookieName() string {
	return s.authConfig.SessionCookieName
}

func (s *Service) SSOStateCookieName() string {
	return s.authConfig.SSOStateCookieName
}

func (s *Service) SessionCookie(token string, secure bool) *http.Cookie {
	return support.ValueCookie(s.authConfig.SessionCookieName, token, secure)
}

func (s *Service) ClearSessionCookie(secure bool) *http.Cookie {
	return support.ClearCookie(s.authConfig.SessionCookieName, secure)
}

// WithSession attaches a validated session to the request context.
func WithSession(ctx context.Context, s *SessionWithUser) context.Context {
	return context.WithValue(ctx, ctxKeySession, s)
}

// SessionFrom retrieves the session from the context, or nil for anonymous requests.
func SessionFrom(ctx context.Context) *SessionWithUser {
	s, _ := ctx.Value(ctxKeySession).(*SessionWithUser)
	return s
}
