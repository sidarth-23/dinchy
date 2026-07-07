// Package support provides shared transport helpers for the HTTP server layer.
package support

import "net/http"

// Cookie names used across the application.
const CSRFCookieName = "dinchy_csrf"

// ValueCookie builds an HttpOnly session-style cookie with a value.
func ValueCookie(name, value string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ClearCookie builds a cookie deletion instruction for a session-style cookie.
func ClearCookie(name string, secure bool) *http.Cookie {
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
