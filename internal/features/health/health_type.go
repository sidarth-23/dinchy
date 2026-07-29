package health

import "context"

// Pinger verifies that a backing service is reachable.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// Check reports the health of a subsystem the process can run without.
//
// A failing Check is surfaced in the readiness payload but never makes the process
// unready. The reverse proxy is the motivating case: when routing breaks, the
// management plane is the only thing that can repair it, so reporting unready — and
// inviting an orchestrator to restart it — would remove the fix.
type Check struct {
	// Name labels the subsystem in the readiness payload.
	Name string
	// Degraded returns a reason when the subsystem is unhealthy, or empty when it is fine.
	// It takes a context because a Check may probe the subsystem over the network, and a
	// readiness request that has gone away must not leave a probe running.
	Degraded func(ctx context.Context) string
}

// LivenessOut is the plain-text response body for the liveness endpoint.
type LivenessOut struct {
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
