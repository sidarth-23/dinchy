package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	mw "github.com/sidarth-23/dinchy/internal/server/middleware"
)

// Pinger verifies that a backing service is reachable.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// NewInternal creates a minimal http.Server for liveness and readiness probes.
// It exposes only /healthz and /readyz with no auth, CSRF, or CORS middleware.
// This server should be bound to an internal-only address.
func NewInternal(addr string, db Pinger) *http.Server {
	r := chi.NewRouter()
	r.Use(mw.RequestID())
	r.Use(mw.Recover())

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		dbErr := db.PingContext(r.Context())

		checks := map[string]string{}
		ready := true

		if dbErr != nil {
			checks["database"] = dbErr.Error()
			ready = false
		} else {
			checks["database"] = "ok"
		}

		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ready":   ready,
			"version": "dev",
			"checks":  checks,
		})
	})

	return &http.Server{
		Addr:    addr,
		Handler: r,
	}
}
