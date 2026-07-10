// Package app wires the application dependencies together and owns the server lifecycle.
package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	goredis "github.com/redis/go-redis/v9"

	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/features/audit"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/health"
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
	workers  *workers.Runtime
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
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageOpenStore))
	}
	a.closer = s
	queries := sqlcgen.New(s.Pool())
	redisClient, err := cache.OpenRedis(ctx, a.cfg.Redis)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
	}
	a.redis = redisClient
	eventBusSvc, err := events.NewService(redisClient, id.NewGenerator(), events.Config{StreamName: a.cfg.EventBus.StreamName, ConsumerGroupPrefix: a.cfg.EventBus.ConsumerGroupPrefix, ConsumerName: a.cfg.EventBus.ConsumerName, BatchSize: a.cfg.EventBus.BatchSize, RetentionWindow: a.cfg.EventBus.RetentionWindow, ClaimMinIdle: a.cfg.EventBus.ClaimMinIdle, ReadBlock: a.cfg.EventBus.ReadBlock, WorkerInterval: a.cfg.EventBus.WorkerInterval})
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
	}
	clk := clock.System{}
	featureDependencies := features.FeatureDependencies{Logger: a.logger}
	serviceDependencies := features.ServiceDependencies{
		FeatureDependencies: featureDependencies,
		Clock:               clk,
		IDGenerator:         id.NewGenerator(),
	}
	auditSvc, err := audit.NewService(audit.Dependencies{Base: serviceDependencies, Store: queries})
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
	}
	eventBusSvc.Register(auditSvc)
	if err := eventBusSvc.EnsureConsumerGroups(ctx); err != nil {
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
	}
	var sender email.Sender = email.NoopSender{}
	if a.cfg.SMTP.Enabled() {
		smtpSender, err := email.NewSMTPSender(a.cfg.SMTP)
		if err != nil {
			return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
		}
		sender = smtpSender
	}
	mailer, err := email.NewMailer(sender, a.cfg.PublicBaseURL)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
	}
	keyer := cache.NewKeyer(a.cfg.Redis.KeyPrefix)
	sessionCache := cache.NewRedis(redisClient, keyer, a.cfg.Cache.Enabled)
	sessionSvc, err := session.NewService(session.Dependencies{Base: serviceDependencies, Store: queries, Config: a.cfg.Session, CacheConfig: a.cfg.Cache, Cache: sessionCache})
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
	}
	authSvc, err := auth.NewService(auth.Dependencies{Base: serviceDependencies, Database: s.Pool(), Store: queries, Sessions: sessionSvc, AuthConfig: a.cfg.Auth, Providers: a.cfg.SSOProviders, RedisClient: redisClient, CacheKeyer: keyer, Mailer: mailer, EventPublisher: eventBusSvc})
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
	}
	var dist fs.FS
	if !a.cfg.DevMode {
		distFS, err := frontend.DistFS()
		if err != nil {
			return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageFrontendDistFs), apperrors.WithStage(apperrors.StageLoadFrontendAssets))
		}
		dist = distFS
	}
	a.public = transport.New(a.cfg.Addr, dist, authSvc, sessionSvc, auditSvc, s, a.cfg.RequireHTTPSForAuth, a.cfg.DevMode, a.cfg.DevProxyURL, a.logger)
	healthAPI := health.NewAPI(health.Dependencies{Base: featureDependencies, DB: s})
	a.internal = transport.NewInternal(a.cfg.InternalAddr, healthAPI)
	registeredWorkers := []workers.Worker{workers.NewSessionCleanupWorker(queries, clk), events.NewWorker(eventBusSvc, auditSvc)}
	a.workers = workers.NewRuntime(queries, clk, a.logger, registeredWorkers...)
	if err := a.workers.Start(ctx); err != nil {
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageStartTaskRuntime))
	}
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
		a.workers.Stop()
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
