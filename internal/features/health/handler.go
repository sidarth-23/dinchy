// Package health serves liveness and readiness probes for the process.
package health

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/sidarth-23/dinchy/internal/features"
)

// API groups the health handlers and their shared dependencies.
type API struct {
	features.BaseFeature
	db Pinger
}

// Dependencies contains the dependencies required by the health API.
type Dependencies struct {
	Base features.FeatureDependencies
	DB   Pinger
}

// NewAPI builds the health API.
func NewAPI(dependencies Dependencies) *API {
	return &API{BaseFeature: features.NewBaseFeature("health", dependencies.Base), db: dependencies.DB}
}

// Register mounts the health API operations on the given huma.API instance.
func (a *API) Register(h huma.API) {
	huma.Register(h, huma.Operation{
		OperationID: "health-healthz",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Liveness probe",
		Description: "Returns a plain-text ok response when the process is running.",
		Tags:        []string{"Health"},
	}, a.healthz)

	huma.Register(h, huma.Operation{
		OperationID: "health-readyz",
		Method:      http.MethodGet,
		Path:        "/readyz",
		Summary:     "Readiness probe",
		Description: "Returns whether the process is ready to serve traffic and includes backend status checks.",
		Tags:        []string{"Health"},
	}, a.readyz)
}

var _ features.Feature = (*API)(nil)

func (a *API) healthz(context.Context, *struct{}) (*HealthOut, error) {
	return &HealthOut{
		ContentType: "text/plain",
		Body:        []byte("ok"),
	}, nil
}

func (a *API) readyz(ctx context.Context, _ *struct{}) (*ReadyzOut, error) {
	ready := true
	checks := map[string]string{"database": "ok"}

	if a.db == nil {
		ready = false
		checks["database"] = "not configured"
	} else if err := a.db.PingContext(ctx); err != nil {
		ready = false
		checks["database"] = err.Error()
	}

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}

	out := &ReadyzOut{
		Status: status,
		Body: ReadyzBody{
			Ready:   ready,
			Version: "dev",
			Checks:  checks,
		},
	}
	return out, nil
}
