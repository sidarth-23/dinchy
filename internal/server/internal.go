package server

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
)

// Pinger verifies that a backing service is reachable.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// NewInternal creates a minimal Echo instance for liveness and readiness probes.
// It exposes only /healthz and /readyz with no auth, CSRF, or CORS middleware.
// This server should be bound to an internal-only address.
func NewInternal(addr string, db Pinger) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	e.Use(echomw.RequestID())
	e.Use(echomw.Recover())

	e.GET("/healthz", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	e.GET("/readyz", func(c echo.Context) error {
		dbErr := db.PingContext(c.Request().Context())

		checks := map[string]string{}
		ready := true

		if dbErr != nil {
			checks["database"] = dbErr.Error()
			ready = false
		} else {
			checks["database"] = "ok"
		}

		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		return c.JSON(status, map[string]any{
			"ready":   ready,
			"version": "dev",
			"checks":  checks,
		})
	})

	e.Server.Addr = addr
	return e
}
