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
	cfg          config.Config
	closer       io.Closer
	echo         *echo.Echo
	internalEcho *echo.Echo
	tasks        *tasks.Runtime
	errCh        chan error
}

// NewApp creates an App with the given configuration. Heavy initialization is deferred to Start.
func NewApp(cfg config.Config) (*App, error) {
	return &App{cfg: cfg, errCh: make(chan error, 2)}, nil
}

// Start initializes all dependencies, starts the task runtime, and begins listening
// on both the public and internal server addresses.
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

	e := server.New(a.cfg.Addr, dist, authSvc, s, a.cfg.RequireHTTPSForAuth, a.cfg.DevMode, a.cfg.DevProxyURL)
	a.echo = e

	ie := server.NewInternal(a.cfg.InternalAddr, s)
	a.internalEcho = ie

	a.tasks = tasks.NewRuntime(s, clk)
	if err := a.tasks.Start(ctx); err != nil {
		return err
	}

	go func() { a.errCh <- e.Start(a.cfg.Addr) }()
	go func() { a.errCh <- ie.Start(a.cfg.InternalAddr) }()
	return nil
}

// Shutdown performs a graceful shutdown in dependency order: tasks first, then
// the public server, then the internal server (kept up during public drain), then
// the database.
func (a *App) Shutdown(ctx context.Context) error {
	if a.tasks != nil {
		a.tasks.Stop()
	}
	if a.echo != nil {
		_ = a.echo.Shutdown(ctx)
	}
	if a.internalEcho != nil {
		_ = a.internalEcho.Shutdown(ctx)
	}
	if a.closer != nil {
		_ = a.closer.Close()
	}
	return nil
}

// Wait blocks until both servers exit and returns the first fatal error encountered.
func (a *App) Wait() error {
	var firstErr error
	for range 2 {
		if err := <-a.errCh; err != nil && err != http.ErrServerClosed {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func frontendFS(devMode bool) (fs.FS, error) {
	if devMode {
		return nil, nil
	}
	return frontend.DistFS()
}
