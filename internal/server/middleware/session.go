package middleware

import (
	"net/http"

	"github.com/sidarth-23/dinchy/internal/auth"
	"github.com/sidarth-23/dinchy/internal/server/support"
)

// Session reads the session cookie, validates it via the auth service, and injects
// the session into the request context. Requests with absent or invalid cookies
// continue as anonymous (nil session).
func Session(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(support.SessionCookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			sess, err := svc.Session(r.Context(), cookie.Value)
			if err != nil || sess == nil {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(support.WithSession(r.Context(), sess)))
		})
	}
}
