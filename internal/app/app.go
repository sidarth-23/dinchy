// Package app wires all dependencies and manages the application lifecycle.
package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/tasks"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/frontend"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/store/sqlite"
	transport "github.com/sidarth-23/dinchy/internal/transport"
)

// App is the top-level application container.
type App struct {
	cfg      config.Config
	closer   io.Closer
	public   *http.Server
	internal *http.Server
	tasks    *tasks.Runtime
	errCh    chan error
}

// NewApp creates an App with the given configuration. Heavy initialization is deferred to Start.
func NewApp(cfg config.Config) (*App, error) {
	return &App{cfg: cfg, errCh: make(chan error, 3)}, nil
}

// Start initializes all dependencies, starts the task runtime, and begins listening
// on both the public and internal server addresses.
func (a *App) Start() error {
	ctx := context.Background()

	s, err := sqlite.Open(ctx, a.cfg.DBPath)
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithStage(apperrors.StageOpenStore),
		)
	}
	a.closer = s

	clk := clock.RealClock{}
	authSvc := auth.NewService(s, id.NewGenerator(), clk)

	dist, err := frontendFS(a.cfg.DevMode)
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithStage(apperrors.StageLoadFrontendAssets),
		)
	}

	a.public = transport.New(a.cfg.Addr, dist, authSvc, s, a.cfg.RequireHTTPSForAuth, a.cfg.DevMode, a.cfg.DevProxyURL)
	a.internal = transport.NewInternal(a.cfg.InternalAddr, s)

	a.tasks = tasks.NewRuntime(s, clk, a.errCh)
	if err := a.tasks.Start(ctx); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithStage(apperrors.StageStartTaskRuntime),
		)
	}

	go func() { a.errCh <- a.public.ListenAndServe() }()
	go func() { a.errCh <- a.internal.ListenAndServe() }()
	return nil
}

// Shutdown performs a graceful shutdown in dependency order: tasks first, then
// the public server, then the internal server (kept up during public drain), then
// the database.
func (a *App) Shutdown(ctx context.Context) error {
	var shutdownErr error
	if a.tasks != nil {
		a.tasks.Stop()
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
	if a.closer != nil {
		if err := a.closer.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
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
			apperrors.WithStage(apperrors.StageFrontendDistFS),
		)
	}
	return dist, nil
}
