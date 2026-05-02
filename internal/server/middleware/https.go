// Package middleware provides Echo middleware for the Dinchy HTTP server.
package middleware

import (
	"github.com/labstack/echo/v4"

	"github.com/sidarth-23/dinchy/internal/server/support"
)

// SecureDetect injects whether the current request arrived over HTTPS into the context.
// Checks direct TLS and the X-Forwarded-Proto header from trusted proxies.
func SecureDetect() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			secure := c.IsTLS() || c.Request().Header.Get("X-Forwarded-Proto") == "https"
			ctx := support.WithSecure(c.Request().Context(), secure)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
