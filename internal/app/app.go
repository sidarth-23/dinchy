// Package app wires all dependencies and manages the application lifecycle.
package app

import (
	"context"
	"io"
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sidarth-23/dinchy/internal/auth"
	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/frontend"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/server"
	"github.com/sidarth-23/dinchy/internal/store/sqlite"
	"github.com/sidarth-23/dinchy/internal/tasks"
)

// App is the top-level application container.
type App struct {
	cfg     config.Config
	closer  io.Closer
	echo    *echo.Echo
	httpSrv *http.Server
	tasks   *tasks.Runtime
	errCh   chan error
}

// NewApp creates an App with the given configuration. Heavy initialisation is deferred to Start.
func NewApp(cfg config.Config) (*App, error) {
	return &App{cfg: cfg, errCh: make(chan error, 1)}, nil
}

// Start initialises all dependencies, starts the task runtime, and begins listening.
func (a *App) Start() error {
	ctx := context.Background()

	s, err := sqlite.Open(ctx, a.cfg.DBPath)
	if err != nil {
		return err
	}
	a.closer = s

	clk := clock.RealClock{}
	authSvc := auth.NewService(s, id.NewGenerator(), clk)

	dist, err := frontendFS(a.cfg.DevMode)
	if err != nil {
		return err
	}

	e := server.New(a.cfg.Addr, dist, authSvc, s, a.cfg.RequireHTTPSForAuth, a.cfg.DevMode)
	a.echo = e
	a.httpSrv = e.Server

	a.tasks = tasks.NewRuntime(s, clk)
	if err := a.tasks.Start(ctx); err != nil {
		return err
	}

	go func() { a.errCh <- e.Start(a.cfg.Addr) }()
	return nil
}

// Shutdown performs a graceful shutdown of all running components.
func (a *App) Shutdown(ctx context.Context) error {
	if a.tasks != nil {
		a.tasks.Stop()
	}
	if a.echo != nil {
		_ = a.echo.Shutdown(ctx)
	}
	if a.closer != nil {
		_ = a.closer.Close()
	}
	return nil
}

// Wait blocks until the HTTP server exits and returns any fatal error.
func (a *App) Wait() error {
	err := <-a.errCh
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func frontendFS(_ bool) (fs.FS, error) {
	return frontend.DistFS()
}
