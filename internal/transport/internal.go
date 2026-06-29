package transport

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/sidarth-23/dinchy/internal/features/health"
	mw "github.com/sidarth-23/dinchy/internal/transport/middleware"
)

// NewInternal creates a minimal http.Server for liveness and readiness probes.
// It serves a separate internal listener and mounts the probes as Huma routes.
func NewInternal(addr string, db health.Pinger) *http.Server {
	r := chi.NewRouter()
	r.Use(mw.RequestID())
	r.Use(mw.Recover())

	apiRouter := chi.NewRouter()
	cfg := huma.DefaultConfig("Dinchy Internal API", "0.1.0")
	cfg.Servers = []*huma.Server{{URL: "/"}}
	api := humachi.New(apiRouter, cfg)
	health.Register(api, db)
	r.Mount("/", apiRouter)

	return &http.Server{
		Addr:    addr,
		Handler: r,
	}
}
