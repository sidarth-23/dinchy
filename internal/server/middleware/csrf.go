package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sidarth-23/dinchy/internal/server/support"
)

// CSRF returns middleware implementing the double-submit cookie pattern.
// Every request ensures a dinchy_csrf cookie exists; mutating requests
// (POST, PUT, PATCH, DELETE) additionally require the X-CSRF-Token header
// to match the cookie value.
func CSRF() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			secure := support.IsSecure(c.Request().Context())

			// Fetch or generate the CSRF token.
			var token string
			cookie, err := c.Cookie(support.CSRFCookieName)
			if err != nil || cookie.Value == "" {
				token = generateToken()
				c.SetCookie(support.CSRFCookie(token, secure))
			} else {
				token = cookie.Value
			}

			// Validate on mutating methods only.
			switch c.Request().Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				header := c.Request().Header.Get("X-CSRF-Token")
				if subtle.ConstantTimeCompare([]byte(token), []byte(header)) != 1 {
					return &echo.HTTPError{
						Code:    http.StatusBadRequest,
						Message: "missing csrf token in request header",
					}
				}
			}

			return next(c)
		}
	}
}

func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("csrf: rand.Read failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
