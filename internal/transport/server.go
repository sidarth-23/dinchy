// Package transport configures the Chi HTTP router, mounts middleware, and wires API routes.
package transport

import (
	"context"
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

	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/features/audit"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	mw "github.com/sidarth-23/dinchy/internal/transport/middleware"
	"github.com/sidarth-23/dinchy/internal/transport/render"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// New creates a fully configured http.Server with middleware, the Huma API,
// and frontend asset serving. Health and readiness endpoints live on the
// internal server created by NewInternal, not here.
func New(addr string, dist fs.FS, authSvc *auth.Service, sessionSvc *session.Service, auditSvc *audit.Service, requireHTTPS, devMode, exposeInternalErrors bool, devProxyURL string, logger *slog.Logger) *http.Server {
	if logger == nil {
		logger = slog.Default()
	}
	renderer := render.NewRenderer(i18n.Default, exposeInternalErrors)
	huma.NewError = func(status int, _ string, errs ...error) huma.StatusError {
		return renderer.ResponseFor(language.English, status, errs...)
	}
	huma.NewErrorWithContext = func(ctx huma.Context, status int, _ string, errs ...error) huma.StatusError {
		for _, err := range errs {
			if err == nil {
				continue
			}
			logging.HTTPError(ctx.Context(), logger, status, "Request failed", err)
			break
		}
		return renderer.ResponseFor(support.LangFrom(ctx.Context()), status, errs...)
	}

	r := chi.NewRouter()

	r.Use(mw.RequestID())
	r.Use(mw.Recover(logger, renderer))
	r.Use(mw.RealIP())
	r.Use(mw.CleanPath())
	r.Use(mw.SecureDetect())
	r.Use(mw.RequestInfo())
	r.Use(mw.Lang(i18n.Default))
	r.Use(mw.SecureHeaders(devMode))
	r.Use(mw.CORS(devMode, devProxyURL))
	r.Use(mw.CSRF(renderer))
	r.Use(session.RequestMiddleware(sessionSvc.SessionCookieName(), sessionSvc.Session))
	r.Use(mw.Timeout(30 * time.Second))
	r.Use(mw.AccessLog(logger))

	apiRouter := chi.NewRouter()
	apiRouter.Use(mw.NoStore())
	cfg := huma.DefaultConfig("Dinchy API", "0.1.0")
	cfg.Servers = []*huma.Server{{URL: "/api"}}
	api := humachi.New(apiRouter, cfg)
	api.UseMiddleware(mw.SessionResolutionGuard(api))
	auth.Register(api, authSvc, sessionSvc, authSvc, requireHTTPS)
	if auditSvc != nil {
		audit.Register(api, auditSvc)
	}
	r.Mount("/api", apiRouter)

	if devMode {
		target, err := url.Parse(devProxyURL)
		if err != nil {
			logging.Warn(
				context.Background(), logger, "Invalid dev proxy URL",
				slog.String("url", devProxyURL),
				slog.Any("error", err),
			)
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
