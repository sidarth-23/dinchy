package middleware

import (
	"net/http"
	"net/url"

	chi_cors "github.com/go-chi/cors"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// CORS enforces same-origin requests in production and allows the configured
// frontend dev server origin in dev mode.
func CORS(devMode bool, devProxyURL string) func(http.Handler) http.Handler {
	allowedDevOrigin := ""
	if devMode {
		allowedDevOrigin = originFromURL(devProxyURL)
	}

	cors := chi_cors.Handler(chi_cors.Options{
		AllowOriginFunc: func(r *http.Request, origin string) bool {
			return isAllowedOrigin(r, origin, allowedDevOrigin)
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
			if origin != "" && !isAllowedOrigin(r, origin, allowedDevOrigin) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			cors(next).ServeHTTP(w, r)
		})
	}
}

func isAllowedOrigin(r *http.Request, origin, allowedDevOrigin string) bool {
	if origin == requestOrigin(r) {
		return true
	}
	return allowedDevOrigin != "" && origin == allowedDevOrigin
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if support.IsSecure(r.Context()) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func originFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
