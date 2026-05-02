// Package main is the entry point for the Dinchy server binary.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/sidarth-23/dinchy/internal/app"
	"github.com/sidarth-23/dinchy/internal/config"
)

func main() {
	cfg := config.FromEnv()
	a, err := app.NewApp(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := a.Start(); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		_ = a.Shutdown(context.Background())
	}()

	if err := a.Wait(); err != nil {
		log.Fatal(err)
	}
}
