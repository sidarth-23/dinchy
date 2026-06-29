// Package support provides shared transport helpers for the HTTP server layer.
package support

import "net/http"

// Cookie names used across the application.
const CSRFCookieName = "dinchy_csrf"

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
