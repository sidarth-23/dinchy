// Package app wires the application dependencies together and owns the server lifecycle.
package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-co-op/gocron/v2"
	"github.com/jackc/pgx/v5"
	goredis "github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/features/audit"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/health"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/events"
	"github.com/sidarth-23/dinchy/internal/platform/jobs"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	"github.com/sidarth-23/dinchy/internal/platform/store"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	transport "github.com/sidarth-23/dinchy/internal/transport"
	"github.com/sidarth-23/dinchy/internal/workers"
)

// App holds the wired dependencies and running servers for one application instance.
type App struct {
	cfg      config.Config
	closer   io.Closer
	redis    *goredis.Client
	public   *http.Server
	internal *http.Server
	workers  gocron.Scheduler
	jobs     *river.Client[pgx.Tx]
	caddy    *caddy.Reconciler
	errCh    chan error
	logger   *slog.Logger
}

// NewApp creates an App from config, defaulting the logger when nil.
func NewApp(cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return &App{cfg: cfg, errCh: make(chan error, 2), logger: logger}, nil
}

// Start opens dependencies, wires services, and begins serving public and internal traffic.
func (a *App) Start() error {
	ctx := context.Background()
	s, err := store.Open(ctx, a.cfg.Database.PostgresDSN, store.WithLogger(a.logger))
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppOpenStore), apperrors.WithCause(err))
	}
	a.closer = s
	if err := jobs.Migrate(ctx, s.Pool(), a.logger); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	queries := sqlcgen.New(s.Pool())
	redisClient, err := cache.OpenRedis(ctx, a.cfg.Redis)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	a.redis = redisClient
	eventBusSvc, err := events.NewService(redisClient, id.NewGenerator(), events.Config{StreamName: a.cfg.EventBus.StreamName, ConsumerGroupPrefix: a.cfg.EventBus.ConsumerGroupPrefix, ConsumerName: a.cfg.EventBus.ConsumerName, BatchSize: a.cfg.EventBus.BatchSize, RetentionWindow: a.cfg.EventBus.RetentionWindow, ClaimMinIdle: a.cfg.EventBus.ClaimMinIdle, ReadBlock: a.cfg.EventBus.ReadBlock, WorkerInterval: a.cfg.EventBus.WorkerInterval})
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	eventBusSvc.RegisterDefinitions(auth.EventDefinitions)
	clk := clock.System{}
	var sender email.Sender = email.NoopSender{}
	if a.cfg.SMTP.Enabled() {
		smtpSender, err := email.NewSMTPSender(a.cfg.SMTP)
		if err != nil {
			return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
		}
		sender = smtpSender
	}
	riverWorkers := river.NewWorkers()
	river.AddWorker(riverWorkers, email.NewSendEmailWorker(sender))
	riverClient, err := jobs.New(s.Pool(), a.logger, a.cfg.Jobs, riverWorkers)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	a.jobs = riverClient
	enqueuer := jobs.NewEnqueuer(riverClient)
	mailer, err := email.NewMailer(enqueuer, a.cfg.SMTP.Enabled())
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	keyer := cache.NewKeyer(a.cfg.Redis.KeyPrefix)
	sharedService := features.Service{BaseLogger: a.logger, Clock: clk, IDGenerator: id.NewGenerator(), Database: s.Pool(), RedisClient: redisClient, CacheKeyer: keyer, Mailer: mailer, EventPublisher: eventBusSvc, Jobs: enqueuer}
	auditSvc, err := audit.NewService(sharedService.Named("audit"), queries)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	eventBusSvc.Register(auditSvc)
	if err := eventBusSvc.EnsureConsumerGroups(ctx); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	sessionSvc, err := session.NewService(sharedService.Named("session"), queries, a.cfg.Session, a.cfg.Cache)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	authSvc, err := auth.NewService(sharedService.Named("auth"), queries, sessionSvc, a.cfg.Auth, config.NewLinks(a.cfg.PublicBaseURL), a.cfg.SSOProviders)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	a.public = transport.New(transport.Options{
		Addr:                 a.cfg.Addr,
		ExposeInternalErrors: a.cfg.ExposeInternalErrors,
		SecureCookies:        a.cfg.SecureCookies,
		PublicScheme:         a.cfg.PublicScheme(),
	}, authSvc, sessionSvc, auditSvc, a.logger)

	if err := a.startCaddy(ctx, clk); err != nil {
		return err
	}

	healthAPI, err := health.NewAPI(sharedService.Named("health"), s, a.caddyHealthCheck())
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	a.internal = transport.NewInternal(a.cfg.InternalAddr, healthAPI)
	scheduler, err := workers.New(a.logger, a.cfg.Worker)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppStartTaskRuntime), apperrors.WithCause(err))
	}
	if err := workers.RegisterSessionCleanup(scheduler, queries, clk); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppStartTaskRuntime), apperrors.WithCause(err))
	}
	if err := events.RegisterWorkers(scheduler, eventBusSvc); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppStartTaskRuntime), apperrors.WithCause(err))
	}
	if err := caddy.RegisterReconcileWorker(scheduler, a.caddy); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppStartTaskRuntime), apperrors.WithCause(err))
	}
	scheduler.Start()
	a.workers = scheduler
	if err := riverClient.Start(ctx); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	go func() { a.errCh <- a.public.ListenAndServe() }()
	go func() { a.errCh <- a.internal.ListenAndServe() }()
	logging.Info(ctx, a.logger, "Application started", slog.String("public_addr", a.cfg.Addr), slog.String("internal_addr", a.cfg.InternalAddr), slog.Bool("dev_mode", a.cfg.DevMode))
	return nil
}

// startCaddy builds the Caddy reconciler and converges the proxy on the routes Dinchy
// owns. Only the panel route exists today; features that own public entrypoints
// register their own caddy.RouteSource here.
//
// A proxy that cannot be reached must not stop the process from starting. Routing being
// broken is exactly when an operator needs the interface that can repair it, so the
// failure is recorded once here and the recurring reconcile job retries until Caddy
// answers. Dinchy keeps serving on its loopback listener throughout, which is what makes
// recovery over an SSH tunnel possible.
func (a *App) startCaddy(ctx context.Context, clk clock.Clock) error {
	if !a.cfg.Caddy.Enabled {
		logging.Info(ctx, a.logger, "Caddy management disabled", slog.String("component", "caddy"))
		return nil
	}

	// A Caddy binary that cannot be run leaves the module set unknown, which disables
	// plugin availability checks rather than rejecting routes that name a plugin.
	modules, err := caddy.ReadModuleSet(ctx, a.cfg.Caddy.Binary)
	if err != nil {
		logging.Warn(ctx, a.logger, "Caddy module list unavailable",
			slog.String("component", "caddy"),
			slog.String("binary", a.cfg.Caddy.Binary),
		)
	}

	reconciler, err := caddy.NewReconciler(a.cfg.Caddy, caddy.NewAdminClient(a.cfg.Caddy), clk, modules)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	reconciler.Register(caddy.NewStaticSource(caddy.PanelOwner, a.panelRoutes()...))
	a.caddy = reconciler
	a.reconcileCaddyAtStartup(ctx)
	return nil
}

// reconcileCaddyAtStartup converges the proxy once and reports the outcome. It returns
// nothing on purpose: startup is the end of the line for this failure, so it is logged
// here and nowhere else, and the recurring reconcile job owns the retry.
func (a *App) reconcileCaddyAtStartup(ctx context.Context) {
	result, err := a.caddy.ReconcileAll(ctx)
	if err != nil {
		logging.Error(ctx, a.logger, "Caddy reconcile at startup failed", err, slog.String("component", "caddy"))
		return
	}
	logging.Info(ctx, a.logger, "Caddy configuration reconciled",
		slog.String("component", "caddy"),
		slog.Int("routes", result.RouteCount),
	)
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

	routes := []caddy.Route{api, web}
	if a.cfg.Caddy.ServesOwnCertificate() {
		for i := range routes {
			routes[i].TLS = caddy.TLSModeFile
			routes[i].CertFile = a.cfg.Caddy.CertFile
			routes[i].KeyFile = a.cfg.Caddy.KeyFile
		}
	}
	return routes
}

// caddyHealthCheck reports proxy health in the readiness payload without making the
// process unready.
func (a *App) caddyHealthCheck() health.Check {
	return health.Check{
		Name: "caddy",
		Degraded: func() string {
			if a.caddy == nil {
				return ""
			}
			if status := a.caddy.Status(); status.Degraded {
				return status.LastError
			}
			return ""
		},
	}
}

// Shutdown stops workers and servers and closes dependencies, joining any errors.
func (a *App) Shutdown(ctx context.Context) error {
	var shutdownErr error
	logging.Info(ctx, a.logger, "Application stopping")
	if a.workers != nil {
		if err := a.workers.ShutdownWithContext(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if a.jobs != nil {
		if err := a.jobs.Stop(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if a.public != nil {
		if err := a.public.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if a.internal != nil {
		if err := a.internal.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if a.closer != nil {
		if err := a.closer.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if shutdownErr == nil {
		logging.Info(ctx, a.logger, "Application stopped")
	}
	return shutdownErr
}

// Wait blocks until both servers stop, returning the first non-graceful error.
func (a *App) Wait() error {
	closed := 0
	for {
		err := <-a.errCh
		if err != nil && err != http.ErrServerClosed {
			return apperrors.Annotate(err)
		}
		closed++
		if closed >= 2 {
			return nil
		}
	}
}
