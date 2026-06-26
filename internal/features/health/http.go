package health

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Pinger verifies that a backing service is reachable.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// HealthOut is the plain-text response body for the liveness endpoint.
type HealthOut struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

// ReadyzBody is the readiness response payload.
type ReadyzBody struct {
	Ready   bool              `json:"ready"`
	Version string            `json:"version"`
	Checks  map[string]string `json:"checks"`
}

// ReadyzOut is the readiness response envelope.
type ReadyzOut struct {
	Status int
	Body   ReadyzBody
}

// API groups the health handlers and their shared dependencies.
type API struct {
	db Pinger
}

// Register mounts the health operations on the given huma.API instance.
func Register(h huma.API, db Pinger) {
	a := &API{db: db}

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
