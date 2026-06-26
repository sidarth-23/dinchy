// Package support provides shared transport helpers for the HTTP server layer.
package support

import "net/http"

// Cookie names used across the application.
const (
	SessionCookieName = "dinchy_session"
	CSRFCookieName    = "dinchy_csrf"
)

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

// CSRFCookie builds the CSRF double-submit cookie.
// Not HttpOnly — the frontend JavaScript must read this value.
func CSRFCookie(token string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}
