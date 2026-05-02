// Package server configures the Echo HTTP server, mounts middleware, and wires API routes.
package server

import (
	"io/fs"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"github.com/sidarth-23/dinchy/internal/auth"
	"github.com/sidarth-23/dinchy/internal/domain"
	serverapi "github.com/sidarth-23/dinchy/internal/server/api"
	mw "github.com/sidarth-23/dinchy/internal/server/middleware"
)

// New creates a fully configured Echo instance with middleware, health endpoints,
// the Huma API, and frontend asset serving.
func New(addr string, dist fs.FS, authSvc *auth.Service, sr domain.SettingsReader, requireHTTPS, devMode bool) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	e.Use(echomw.RequestID())
	e.Use(echomw.Recover())
	e.Use(mw.SecureDetect())
	e.Use(mw.RequestInfo())
	e.Use(mw.SecureHeaders(devMode))
	e.Use(echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
	}))
	e.Use(mw.CSRF())
	e.Use(mw.Session(authSvc))

	e.GET("/healthz", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/readyz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{
			"ready":   true,
			"version": "dev",
			"checks": map[string]bool{
				"database":       true,
				"migrations":     true,
				"frontend_embed": true,
			},
		})
	})

	cfg := huma.DefaultConfig("Dinchy API", "0.1.0")
	cfg.Servers = []*huma.Server{{URL: "/api"}}
	g := e.Group("/api")
	api := humaecho.NewWithGroup(e, g, cfg)
	serverapi.Register(api, authSvc, sr, requireHTTPS)

	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(dist))))
	e.Server.Addr = addr
	return e
}
