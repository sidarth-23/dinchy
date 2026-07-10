package middleware

import "net/http"

// NoStore returns a middleware that marks responses as uncacheable by shared
// caches and reverse proxies. API responses carry per-user data, so no
// intermediary (Cloudflare, nginx, and the like) may store or reuse them.
func NoStore() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "private, no-store")
			next.ServeHTTP(w, r)
		})
	}
}
