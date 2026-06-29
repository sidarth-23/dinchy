package auth

import (
	"context"
	"net/http"
)

// Cookie names used by auth.
const SessionCookieName = "dinchy_session"

// SessionCookie builds the session cookie with all required security attributes.
func SessionCookie(token string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ClearSessionCookie returns a cookie that immediately expires the session cookie.
func ClearSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
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
