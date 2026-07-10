// Package health serves liveness and readiness probes for the process.
package health

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/module"
)

// API groups the health handlers and their shared dependencies.
type API struct {
	*module.Service
	db Pinger
}

// NewAPI builds the health API.
func NewAPI(base *module.Service, db Pinger) (*API, error) {
	if base == nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(errors.New("health module service is required")))
	}
	if err := base.Initialize(); err != nil {
		return nil, apperrors.Annotate(err)
	}
	return &API{Service: base, db: db}, nil
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

var _ module.Module = (*API)(nil)

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
