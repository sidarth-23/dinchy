package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/sidarth-23/dinchy/internal/features/health"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	transport "github.com/sidarth-23/dinchy/internal/transport"
)

// caddyStartupAttempts and caddyStartupBackoff bound the wait for Caddy to come up.
// systemd orders Caddy first, so this covers the moment between its process starting and
// its admin endpoint accepting connections — not an outage.
const (
	caddyStartupAttempts = 5
	caddyStartupBackoff  = 2 * time.Second
)

// startCaddy builds the Caddy reconciler and converges the proxy on the routes Dinchy
// owns. Only the panel route exists today; features that own public entrypoints
// register their own caddy.RouteSource here.
//
// A proxy that cannot be reached must not stop the process from starting. Routing being
// broken is exactly when an operator needs the interface that can repair it, so the
// failure is recorded once here and Dinchy keeps serving on its loopback listener, which
// is what makes recovery over an SSH tunnel possible.
func (a *App) startCaddy(ctx context.Context) error {
	if !a.cfg.Caddy.Enabled {
		logging.Info(ctx, a.logger, "Caddy management disabled", slog.String("component", "caddy"))
		return nil
	}

	reconciler, err := caddy.NewReconciler(a.cfg.Caddy, caddy.NewAdminClient(a.cfg.Caddy))
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	reconciler.Register(caddy.NewStaticSource(caddy.PanelOwner, a.panelRoutes()...))
	a.caddy = reconciler
	a.reconcileCaddyAtStartup(ctx)
	return nil
}

// reconcileCaddyAtStartup converges the proxy once and reports the outcome.
//
// This is the only time Dinchy writes to Caddy. It does not re-assert on a timer: the
// operator owns the running configuration once it is set up, and a management plane that
// silently overwrites their changes every minute is one they cannot work with.
//
// It returns nothing on purpose: startup is the end of the line for this failure, so it is
// logged here and nowhere else.
func (a *App) reconcileCaddyAtStartup(ctx context.Context) {
	var err error
	for attempt := 1; ; attempt++ {
		var result caddy.Result
		result, err = a.caddy.ReconcileAll(ctx)
		if err == nil {
			logging.Info(ctx, a.logger, "Caddy configuration reconciled",
				slog.String("component", "caddy"),
				slog.Int("routes", result.RouteCount),
			)
			return
		}
		// Only an unreachable admin endpoint is worth waiting on. A configuration Caddy
		// rejected will be rejected identically on every retry.
		if attempt == caddyStartupAttempts || !isCaddyUnreachable(err) {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(caddyStartupBackoff):
		}
	}
	logging.Error(ctx, a.logger, "Caddy reconcile at startup failed", err, slog.String("component", "caddy"))
}

// isCaddyUnreachable reports whether the failure was the admin endpoint not answering,
// as opposed to Caddy answering and refusing the configuration.
func isCaddyUnreachable(err error) bool {
	appErr, ok := errors.AsType[*apperrors.AppError](err)
	return ok && appErr.Code() == i18n.CodePlatformRoutingUnavailable
}

// panelRoutes describes the entrypoints serving Dinchy's own API and web UI, on one
// hostname split by path.
//
// The web assets do not pass through Dinchy: Caddy reads them from disk in production and
// proxies to the Vite dev server in development, so the Go process only ever answers the
// API. Keeping one hostname keeps the browser same-origin, which is what lets the session
// and CSRF cookies stay SameSite=Lax and keeps CORS out of the picture.
//
// The app layer composes these because they span the listen address, the proxy settings
// and the frontend location, the same way it composes the outbound email links.
func (a *App) panelRoutes() []caddy.Route {
	api := caddy.Route{
		Owner:      caddy.PanelOwner,
		Host:       a.cfg.Caddy.PanelHost,
		PathPrefix: transport.APIPathPrefix,
		Upstream:   a.cfg.Addr,
	}

	web := caddy.Route{
		Owner: caddy.PanelOwner,
		Host:  a.cfg.Caddy.PanelHost,
	}
	if a.cfg.DevMode {
		// Vite serves the assets and its own HMR socket; Caddy forwards to it.
		web.Upstream = a.cfg.FrontendUpstream()
	} else {
		web.Serve = caddy.ServeModeFiles
		web.Root = a.cfg.FrontendRoot
		// Client-side routes have no file behind them, so an unmatched path must serve
		// the application document rather than 404.
		web.FallbackPath = caddy.SPAFallbackPath
	}

	return []caddy.Route{api, web}
}

// caddyHealthCheck reports proxy health in the readiness payload without making the
// process unready. It probes Caddy rather than reporting the startup outcome, which would
// say nothing about the proxy's state minutes later.
func (a *App) caddyHealthCheck() health.Check {
	return health.Check{
		Name: "caddy",
		Degraded: func(ctx context.Context) string {
			if a.caddy == nil {
				return ""
			}
			if err := a.caddy.Ping(ctx); err != nil {
				return err.Error()
			}
			return ""
		},
	}
}
