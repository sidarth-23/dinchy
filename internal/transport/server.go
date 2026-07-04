// Package transport configures the Chi HTTP router, mounts middleware, and wires API routes.
package transport

import (
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/text/language"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/audit"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/i18n"
	mw "github.com/sidarth-23/dinchy/internal/transport/middleware"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// New creates a fully configured http.Server with middleware, the Huma API,
// and frontend asset serving. Health and readiness endpoints live on the
// internal server created by NewInternal, not here.
func New(addr string, dist fs.FS, authSvc *auth.Service, auditSvc *audit.Service, sr auth.SettingsReader, requireHTTPS, devMode bool, devProxyURL string, logger *slog.Logger) *http.Server {
	if logger == nil {
		logger = slog.Default()
	}
	huma.NewError = func(status int, _ string, errs ...error) huma.StatusError {
		return apperrors.ResponseFor(language.English, i18n.Default, status, errs...)
	}
	huma.NewErrorWithContext = func(ctx huma.Context, status int, _ string, errs ...error) huma.StatusError {
		return apperrors.ResponseFor(support.LangFrom(ctx.Context()), i18n.Default, status, errs...)
	}

	r := chi.NewRouter()

	r.Use(mw.RequestID())
	r.Use(mw.Recover())
	r.Use(mw.RealIP())
	r.Use(mw.CleanPath())
	r.Use(mw.SecureDetect())
	r.Use(mw.RequestInfo())
	r.Use(mw.Lang(i18n.Default))
	r.Use(mw.SecureHeaders(devMode))
	r.Use(mw.CORS(devMode, devProxyURL))
	r.Use(mw.CSRF())
	r.Use(mw.Session(authSvc))
	r.Use(mw.Timeout(30 * time.Second))
	r.Use(mw.AccessLog(logger))

	apiRouter := chi.NewRouter()
	cfg := huma.DefaultConfig("Dinchy API", "0.1.0")
	cfg.Servers = []*huma.Server{{URL: "/api"}}
	api := humachi.New(apiRouter, cfg)
	auth.Register(api, authSvc, sr, requireHTTPS)
	audit.Register(api, auditSvc)
	r.Mount("/api", apiRouter)

	if devMode {
		target, err := url.Parse(devProxyURL)
		if err != nil {
			logger.Error("invalid dev proxy url", slog.String("url", devProxyURL), slog.Any("error", err))
			r.Handle("/*", http.NotFoundHandler())
		} else {
			proxy := httputil.NewSingleHostReverseProxy(target)
			r.Handle("/*", proxy)
		}
	} else {
		r.Handle("/*", http.FileServer(http.FS(dist)))
	}

	return &http.Server{
		Addr:    addr,
		Handler: otelhttp.NewHandler(r, "http.server"),
	}
}
