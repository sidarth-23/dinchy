package middleware

// IMPORTANT: This file keeps a few startup-only diagnostic literals.
// They are internal failure details only and are never returned to users.

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// CSRF returns middleware implementing the double-submit cookie pattern.
// Every request ensures a dinchy_csrf cookie exists; mutating requests
// (POST, PUT, PATCH, DELETE) additionally require the X-CSRF-Token header
// to match the cookie value.
func CSRF() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			secure := support.IsSecure(ctx)

			var token string
			cookie, err := r.Cookie(support.CSRFCookieName)
			if err != nil || cookie.Value == "" {
				token, err = security.RandomToken(32)
				if err != nil {
					panic(err)
				}
				http.SetCookie(w, support.CSRFCookie(token, secure))
			} else {
				token = cookie.Value
			}

			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				header := r.Header.Get("X-CSRF-Token")
				if subtle.ConstantTimeCompare([]byte(token), []byte(header)) != 1 {
					locErr := apperrors.Resolve(support.LangFrom(ctx), i18n.Default, apperrors.BadRequest(i18n.Msg(i18n.CodeSecurityCSRFFailed)))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(locErr.GetStatus())
					if err := json.NewEncoder(w).Encode(locErr); err != nil {
						log.Printf("failed to encode CSRF error response: %v", err)
					}
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
