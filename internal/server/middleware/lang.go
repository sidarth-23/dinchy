package middleware

import (
	"github.com/labstack/echo/v4"

	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/server/support"
)

// Lang parses the Accept-Language request header, matches it against the catalog's
// supported locales, and injects the resolved language tag into the request context.
func Lang(catalog *i18n.Catalog) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			accept := c.Request().Header.Get("Accept-Language")
			tag := catalog.Match(accept)
			ctx := support.WithLang(c.Request().Context(), tag)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
