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
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	a, err := app.NewApp(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := a.Start(); err != nil {
		log.Fatal(err)
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
			log.Fatal(err)
		}
	case <-ctx.Done():
		if err := a.Shutdown(context.Background()); err != nil {
			log.Fatal(err)
		}
	}
}
