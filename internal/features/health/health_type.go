package health

import "context"

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
