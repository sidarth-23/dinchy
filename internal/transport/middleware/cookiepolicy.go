package middleware

import (
	"net/http"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// CookiePolicy records whether cookies must carry the Secure attribute.
//
// It is configured once for the installation rather than derived per request. See
// support.SecureCookies for why: cookies ignore port and scheme, so letting one
// plaintext request mint a non-Secure cookie would clobber the Secure one of the same
// name and silently end the session.
func CookiePolicy(secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(support.WithSecureCookies(r.Context(), secureCookies)))
		})
	}
}
