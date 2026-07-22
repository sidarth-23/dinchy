// Package session owns authenticated request principals, session cookies, and session request middleware.
package session

import (
	"context"
	"net/http"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// RequestMiddleware resolves the session cookie and attaches the authenticated principal to the request context.
func RequestMiddleware(cookieName string, resolve func(context.Context, string) (*Principal, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := support.CookieValueFrom(r.Context(), cookieName)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			principal, err := resolve(r.Context(), token)
			if err != nil {
				next.ServeHTTP(w, r.WithContext(WithResolutionError(r.Context(), err)))
				return
			}
			if principal == nil {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
		})
	}
}
