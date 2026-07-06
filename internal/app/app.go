// Package app wires all dependencies and manages the application lifecycle.
package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/audit"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	cachecore "github.com/sidarth-23/dinchy/internal/platform/cache/core"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/eventbus"
	"github.com/sidarth-23/dinchy/internal/platform/frontend"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	"github.com/sidarth-23/dinchy/internal/platform/store"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	transport "github.com/sidarth-23/dinchy/internal/transport"
	"github.com/sidarth-23/dinchy/internal/workers"
)

// App is the top-level application container.
type App struct {
	cfg      config.Config
	closer   io.Closer
	cache    cache.Store
	public   *http.Server
	internal *http.Server
	workers  *workers.Runtime
	errCh    chan error
	logger   *slog.Logger
}

// NewApp creates an App with the given configuration. Heavy initialization is deferred to Start.
func NewApp(cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return &App{cfg: cfg, errCh: make(chan error, 3), logger: logger}, nil
}

// Start initializes all dependencies, starts the worker runtime, and begins listening
// on both the public and internal server addresses.
func (a *App) Start() error {
	ctx := context.Background()

	s, err := store.Open(ctx, a.cfg.Database.PostgresDSN, store.WithLogger(a.logger))
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithStage(apperrors.StageOpenStore),
		)
	}
	a.closer = s
	queries := sqlcgen.New(s.Pool())

	cacheStore, err := cache.Open(ctx, a.cfg.Cache)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
	}
	a.cache = cacheStore
	streamStore, _ := cacheStore.(cachecore.StreamStore)
	var eventBusSvc *eventbus.Service
	var auditSvc *audit.Service
	if a.cfg.Audit.Enabled {
		if streamStore == nil {
			return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(errors.New("redis stream store is required when audit is enabled")))
		}
		eventBusSvc, err = eventbus.NewService(streamStore, id.NewGenerator(), eventbus.Config{
			StreamName:          a.cfg.EventBus.StreamName,
			ConsumerGroupPrefix: a.cfg.EventBus.ConsumerGroupPrefix,
			ConsumerName:        a.cfg.EventBus.ConsumerName,
			BatchSize:           a.cfg.EventBus.BatchSize,
			RetentionWindow:     a.cfg.EventBus.RetentionWindow,
			ClaimMinIdle:        a.cfg.EventBus.ClaimMinIdle,
			ReadBlock:           a.cfg.EventBus.ReadBlock,
			WorkerInterval:      a.cfg.EventBus.WorkerInterval,
		})
		if err != nil {
			return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
		}
		auditSvc, err = audit.NewService(queries)
		if err != nil {
			return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
		}
		eventBusSvc.Register(auditSvc)
		if err := eventBusSvc.EnsureConsumerGroups(ctx); err != nil {
			return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
		}
	}

	clk := clock.RealClock{}
	var sender email.Sender = email.NoopSender{}
	if a.cfg.SMTP.Enabled() {
		smtpSender, err := email.NewSMTPSender(a.cfg.SMTP)
		if err != nil {
			return apperrors.Annotate(err, apperrors.WithStage(apperrors.StageSetup))
		}
		sender = smtpSender
	}
	authSvc, err := auth.NewService(s.Pool(), queries, id.NewGenerator(), clk, a.cfg.Auth, a.cfg.SSOProviders, cacheStore, cachecore.NewKeyer(a.cfg.Cache.KeyPrefix), sender, eventBusSvc)
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithStage(apperrors.StageSetup),
		)
	}

	dist, err := frontendFS(a.cfg.DevMode)
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithStage(apperrors.StageLoadFrontendAssets),
		)
	}

	a.public = transport.New(a.cfg.Addr, dist, authSvc, auditSvc, s, a.cfg.RequireHTTPSForAuth, a.cfg.DevMode, a.cfg.DevProxyURL, a.logger)
	a.internal = transport.NewInternal(a.cfg.InternalAddr, s)

	registeredWorkers := []workers.Worker{workers.NewSessionCleanupWorker(queries, clk)}
	if eventBusSvc != nil {
		registeredWorkers = append(registeredWorkers, eventbus.NewWorker(eventBusSvc, auditSvc.Name()))
	}
	a.workers = workers.NewRuntime(queries, clk, a.logger, a.errCh, registeredWorkers...)
	if err := a.workers.Start(ctx); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithStage(apperrors.StageStartTaskRuntime),
		)
	}

	go func() { a.errCh <- a.public.ListenAndServe() }()
	go func() { a.errCh <- a.internal.ListenAndServe() }()
	logging.Info(ctx, a.logger, "Application started",
		slog.String("public_addr", a.cfg.Addr),
		slog.String("internal_addr", a.cfg.InternalAddr),
		slog.Bool("dev_mode", a.cfg.DevMode),
	)
	return nil
}

// Shutdown performs a graceful shutdown in dependency order: workers first, then
// the public server, then the internal server (kept up during public drain), then
// the database.
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
	if a.cache != nil {
		if err := a.cache.Close(); err != nil {
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

// Wait blocks until both servers exit and returns the first fatal error encountered.
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

func frontendFS(devMode bool) (fs.FS, error) {
	if devMode {
		return nil, nil
	}
	dist, err := frontend.DistFS()
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithStage(apperrors.StageFrontendDistFs),
		)
	}
	return dist, nil
}
