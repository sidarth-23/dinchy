package auth

import (
	"context"
	"net/http"
)

func (s *Service) SessionCookieName() string {
	return s.authConfig.SessionCookieName
}

func (s *Service) SSOStateCookieName() string {
	return s.authConfig.SSOStateCookieName
}

func (s *Service) SessionCookie(token string, secure bool) *http.Cookie {
	return sessionCookie(s.authConfig.SessionCookieName, token, secure)
}

func (s *Service) ClearSessionCookie(secure bool) *http.Cookie {
	return clearSessionCookie(s.authConfig.SessionCookieName, secure)
}

func sessionCookie(name, token string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func clearSessionCookie(name string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

type ctxKey int

const ctxKeySession ctxKey = iota

// WithSession attaches a validated session to the request context.
func WithSession(ctx context.Context, s *SessionWithUser) context.Context {
	return context.WithValue(ctx, ctxKeySession, s)
}

// SessionFrom retrieves the session from the context, or nil for anonymous requests.
func SessionFrom(ctx context.Context) *SessionWithUser {
	s, _ := ctx.Value(ctxKeySession).(*SessionWithUser)
	return s
}
