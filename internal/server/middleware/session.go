package middleware

import (
	"github.com/labstack/echo/v4"
	"github.com/sidarth-23/dinchy/internal/auth"
	"github.com/sidarth-23/dinchy/internal/server/support"
)

// Session reads the session cookie, validates it via the auth service, and injects
// the session into the request context. Requests with absent or invalid cookies
// continue as anonymous (nil session).
func Session(svc *auth.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookie, err := c.Cookie(support.SessionCookieName)
			if err != nil || cookie.Value == "" {
				return next(c)
			}
			sess, err := svc.Session(c.Request().Context(), cookie.Value)
			if err != nil || sess == nil {
				return next(c)
			}
			ctx := support.WithSession(c.Request().Context(), sess)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
