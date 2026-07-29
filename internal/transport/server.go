// Package transport configures the Chi HTTP router, mounts middleware, and wires API routes.
package transport

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/text/language"

	"github.com/sidarth-23/dinchy/internal/features/audit"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	mw "github.com/sidarth-23/dinchy/internal/transport/middleware"
	"github.com/sidarth-23/dinchy/internal/transport/render"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// APIPathPrefix is the path every API operation is mounted under. Caddy routes this
// prefix to Dinchy and everything else to the web assets, so it is the seam between the
// two halves of one origin.
const APIPathPrefix = "/api"

// Options carries the non-service settings the public server needs. They are grouped
// so the several values are named at the call site instead of being positional.
type Options struct {
	// Addr is the plaintext listen address; Caddy proxies to it.
	Addr string
	// ExposeInternalErrors adds internal failure detail to error responses.
	ExposeInternalErrors bool
	// SecureCookies marks every cookie Secure.
	SecureCookies bool
	// PublicScheme is the scheme users reach the app on, used to build the expected
	// CORS origin. It comes from configuration because Caddy terminates TLS, so the
	// request itself cannot report the browser's scheme.
	PublicScheme string
}

// New creates a fully configured http.Server serving the API. It does not serve the web
// UI: Caddy delivers those assets directly, so nothing here handles a document request.
// Health and readiness endpoints live on the internal server created by NewInternal.
func New(opts Options, authSvc *auth.Service, sessionSvc *session.Service, auditSvc *audit.Service, logger *slog.Logger) *http.Server {
	if logger == nil {
		logger = slog.Default()
	}
	renderer := render.NewRenderer(i18n.Default, opts.ExposeInternalErrors)
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
	r.Use(mw.CleanPath())
	r.Use(mw.CookiePolicy(opts.SecureCookies))
	r.Use(mw.RequestInfo())
	r.Use(mw.Lang(i18n.Default))
	r.Use(mw.SecureHeaders())
	r.Use(mw.CORS(opts.PublicScheme))
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
	auth.Register(api, authSvc, sessionSvc, authSvc)
	if auditSvc != nil {
		audit.Register(api, auditSvc)
	}
	r.Mount(APIPathPrefix, apiRouter)

	// Anything outside the API prefix is Caddy's to serve. Reaching this handler means a
	// request bypassed Caddy or its routing is wrong, so answer plainly rather than
	// pretending to be a web server.
	r.Handle("/*", http.NotFoundHandler())

	return &http.Server{
		Addr:    opts.Addr,
		Handler: otelhttp.NewHandler(r, "http.server"),
	}
}
