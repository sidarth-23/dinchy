// Package main is the entry point for the Dinchy server binary.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sidarth-23/dinchy/internal/app"
	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	"github.com/sidarth-23/dinchy/internal/platform/telemetry"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		slog.Default().Error("load configuration", slog.Any("error", err))
		return 1
	}
	telemetryRuntime, err := telemetry.New(context.Background(), cfg.Telemetry)
	if err != nil {
		slog.Default().Error("initialize telemetry", slog.Any("error", err))
		return 1
	}
	defer func() {
		if err := telemetryRuntime.Close(); err != nil {
			slog.Default().Error("shutdown telemetry", slog.Any("error", err))
		}
	}()

	logger := logging.New(cfg.Logging, telemetryRuntime.LogHandler)
	slog.SetDefault(logger)
	a, err := app.NewApp(cfg, logger)
	if err != nil {
		logger.Error("create app", slog.Any("error", err))
		return 1
	}
	if err := a.Start(); err != nil {
		logger.Error("start app", slog.Any("error", err))
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- a.Wait()
	}()

	select {
	case err := <-waitCh:
		if err != nil {
			logger.Error("server stopped", slog.Any("error", err))
			return 1
		}
	case <-ctx.Done():
		if err := a.Shutdown(context.Background()); err != nil {
			logger.Error("shutdown app", slog.Any("error", err))
			return 1
		}
	}
	return 0
}
