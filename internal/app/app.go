// Package app wires the application dependencies together and owns the server lifecycle.
package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-co-op/gocron/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/features/audit"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/health"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/module"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/frontend"
	"github.com/sidarth-23/dinchy/internal/platform/id"
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
	clk := clock.System{}
	var sender email.Sender = email.NoopSender{}
	if a.cfg.SMTP.Enabled() {
		smtpSender, err := email.NewSMTPSender(a.cfg.SMTP)
		if err != nil {
			return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
		}
		sender = smtpSender
	}
	mailer, err := email.NewMailer(sender, a.cfg.PublicBaseURL)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	keyer := cache.NewKeyer(a.cfg.Redis.KeyPrefix)
	sessionCache := cache.NewRedis(redisClient, keyer, a.cfg.Cache.Enabled)
	sharedService := module.Service{BaseLogger: a.logger, Clock: clk, IDGenerator: id.NewGenerator(), Database: s.Pool(), RedisClient: redisClient, Cache: sessionCache, CacheKeyer: keyer, Mailer: mailer, EventPublisher: eventBusSvc}
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
	authSvc, err := auth.NewService(sharedService.Named("auth"), queries, sessionSvc, a.cfg.Auth, a.cfg.SSOProviders)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppSetup), apperrors.WithCause(err))
	}
	var dist fs.FS
	if !a.cfg.DevMode {
		distFS, err := frontend.DistFS()
		if err != nil {
			return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAppLoadFrontendAssets), apperrors.WithCause(err))
		}
		dist = distFS
	}
	a.public = transport.New(a.cfg.Addr, dist, authSvc, sessionSvc, auditSvc, s, a.cfg.RequireHTTPSForAuth, a.cfg.DevMode, a.cfg.ExposeInternalErrors, a.cfg.DevProxyURL, a.logger)
	healthAPI, err := health.NewAPI(sharedService.Named("health"), s)
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
	scheduler.Start()
	a.workers = scheduler
	go func() { a.errCh <- a.public.ListenAndServe() }()
	go func() { a.errCh <- a.internal.ListenAndServe() }()
	logging.Info(ctx, a.logger, "Application started", slog.String("public_addr", a.cfg.Addr), slog.String("internal_addr", a.cfg.InternalAddr), slog.Bool("dev_mode", a.cfg.DevMode))
	return nil
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
