package transport

import (
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/health"
	"github.com/sidarth-23/dinchy/internal/i18n"
	mw "github.com/sidarth-23/dinchy/internal/transport/middleware"
)

// NewInternal creates a minimal http.Server for liveness and readiness probes.
// It serves a separate internal listener and mounts the probes as Huma routes.
func NewInternal(addr string, healthAPI *health.API) *http.Server {
	r := chi.NewRouter()
	r.Use(mw.RequestID())
	r.Use(mw.Recover(slog.Default(), apperrors.NewRenderer(i18n.Default, false)))

	apiRouter := chi.NewRouter()
	cfg := huma.DefaultConfig("Dinchy Internal API", "0.1.0")
	cfg.Servers = []*huma.Server{{URL: "/"}}
	api := humachi.New(apiRouter, cfg)
	if healthAPI != nil {
		healthAPI.Register(api)
	}
	r.Mount("/", apiRouter)

	return &http.Server{
		Addr:    addr,
		Handler: r,
	}
}
