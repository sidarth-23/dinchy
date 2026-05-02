package middleware

import (
	"github.com/labstack/echo/v4"
	"github.com/sidarth-23/dinchy/internal/server/support"
)

// RequestInfo injects the resolved client IP address and User-Agent into the request context.
func RequestInfo() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := support.WithRequestInfo(
				c.Request().Context(),
				c.RealIP(),
				c.Request().UserAgent(),
			)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
