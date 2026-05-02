package middleware

import (
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
)

// SecureHeaders returns a middleware that applies baseline browser security headers.
// devMode enables a relaxed Content-Security-Policy to accommodate Vite HMR.
func SecureHeaders(devMode bool) echo.MiddlewareFunc {
	csp := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'"
	if devMode {
		csp = "default-src 'self' ws: wss:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'"
	}
	return echomw.SecureWithConfig(echomw.SecureConfig{
		XSSProtection:         "",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		ContentSecurityPolicy: csp,
		ReferrerPolicy:        "no-referrer",
	})
}
