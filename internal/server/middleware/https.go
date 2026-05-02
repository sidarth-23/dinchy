package middleware

import (
	"net/http"

	"github.com/sidarth-23/dinchy/internal/server/support"
)

// SecureDetect injects whether the current request arrived over HTTPS into the context.
// Checks direct TLS and the X-Forwarded-Proto header from trusted proxies.
func SecureDetect() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
			next.ServeHTTP(w, r.WithContext(support.WithSecure(r.Context(), secure)))
		})
	}
}
