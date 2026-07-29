package middleware

import (
	"net/http"

	chi_cors "github.com/go-chi/cors"
)

// CORS rejects requests carrying a foreign Origin.
//
// Caddy serves the web UI and the API under one hostname, split by path, so the browser is
// always same-origin and no cross-origin request is legitimate. There is deliberately no
// allowance for the Vite dev server: in development Caddy forwards to Vite on the same
// hostname, so the browser's origin is the panel's, not Vite's.
//
// publicScheme is the scheme users reach the app on, taken from configuration rather than
// from the request. Caddy terminates TLS and proxies plaintext, so the request cannot say
// what scheme the browser used, while the Origin header carries the external scheme it has
// to be compared against.
func CORS(publicScheme string) func(http.Handler) http.Handler {
	cors := chi_cors.Handler(chi_cors.Options{
		AllowOriginFunc: func(r *http.Request, origin string) bool {
			return isAllowedOrigin(r, origin, publicScheme)
		},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-CSRF-Token",
		},
		AllowCredentials: true,
		MaxAge:           600,
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && !isAllowedOrigin(r, origin, publicScheme) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			cors(next).ServeHTTP(w, r)
		})
	}
}

// isAllowedOrigin accepts only the app's own origin.
//
// Host comes from the request because Caddy forwards the original one, port included, so a
// panel reached on a non-default port still matches itself.
func isAllowedOrigin(r *http.Request, origin, publicScheme string) bool {
	return origin == publicScheme+"://"+r.Host
}
