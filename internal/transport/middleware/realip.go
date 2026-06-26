package middleware

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RealIP sets r.RemoteAddr to the result of parsing X-Real-IP or X-Forwarded-For headers.
// Must run before RequestInfo so the resolved IP is available downstream.
func RealIP() func(http.Handler) http.Handler {
	return chimw.RealIP
}
