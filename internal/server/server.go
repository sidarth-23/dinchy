// Package server configures the Echo HTTP server, mounts middleware, and wires API routes.
package server

import (
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"github.com/sidarth-23/dinchy/internal/auth"
	"github.com/sidarth-23/dinchy/internal/domain"
	"github.com/sidarth-23/dinchy/internal/i18n"
	serverapi "github.com/sidarth-23/dinchy/internal/server/api"
	"github.com/sidarth-23/dinchy/internal/server/apierr"
	mw "github.com/sidarth-23/dinchy/internal/server/middleware"
)

// New creates a fully configured Echo instance with middleware, the Huma API,
// and frontend asset serving. Health and readiness endpoints live on the
// internal server created by NewInternal, not here.
func New(addr string, dist fs.FS, authSvc *auth.Service, sr domain.SettingsReader, requireHTTPS, devMode bool, devProxyURL string) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if locErr, ok := err.(*apierr.LocalizedError); ok {
			_ = c.JSON(locErr.GetStatus(), locErr)
			return
		}
		e.DefaultHTTPErrorHandler(err, c)
	}

	e.Use(echomw.RequestID())
	e.Use(echomw.Recover())
	e.Use(mw.SecureDetect())
	e.Use(mw.RequestInfo())
	e.Use(mw.Lang(i18n.Default))
	e.Use(mw.SecureHeaders(devMode))
	e.Use(echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
	}))
	e.Use(mw.CSRF())
	e.Use(mw.Session(authSvc))

	cfg := huma.DefaultConfig("Dinchy API", "0.1.0")
	cfg.Servers = []*huma.Server{{URL: "/api"}}
	g := e.Group("/api")
	api := humaecho.NewWithGroup(e, g, cfg)
	serverapi.Register(api, authSvc, sr, requireHTTPS)

	if devMode && devProxyURL != "" {
		target, _ := url.Parse(devProxyURL)
		proxy := httputil.NewSingleHostReverseProxy(target)
		e.GET("/*", echo.WrapHandler(proxy))
	} else {
		e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(dist))))
	}

	e.Server.Addr = addr
	return e
}
