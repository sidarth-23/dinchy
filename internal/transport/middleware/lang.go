package middleware

import (
	"net/http"

	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// Lang parses the Accept-Language request header, matches it against the catalog's
// supported locales, and injects the resolved language tag into the request context.
func Lang(catalog *i18n.Catalog) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tag := catalog.Match(r.Header.Get("Accept-Language"))
			next.ServeHTTP(w, r.WithContext(support.WithLang(r.Context(), tag)))
		})
	}
}
