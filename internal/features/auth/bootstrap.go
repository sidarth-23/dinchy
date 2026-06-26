package auth

import "context"

type BootstrapState struct {
	SetupRequired bool
	InstanceName  string
}

type SettingsReader interface {
	Bootstrap(ctx context.Context) (BootstrapState, error)
}

// ViewerOut is the current authenticated user projection returned in bootstrap responses.
type ViewerOut struct {
	Email       string `json:"email" doc:"User email address"`
	DisplayName string `json:"display_name" doc:"User display name"`
	Role        string `json:"role" doc:"User role"`
}

// AppOut contains application-level metadata returned in every API response body.
type AppOut struct {
	InstanceName string `json:"instance_name" doc:"Name of this Dinchy instance"`
}

// BootstrapBody is the shared response body for bootstrap, session, login, and setup endpoints.
type BootstrapBody struct {
	SetupRequired bool       `json:"setup_required" doc:"True when no users exist and first-user setup must be completed"`
	Authenticated bool       `json:"authenticated" doc:"True when the request carries a valid session cookie"`
	App           AppOut     `json:"app" doc:"Application-level metadata"`
	Viewer        *ViewerOut `json:"viewer" doc:"Current authenticated user, or null when not authenticated"`
}

// BootstrapOut is the response type for the bootstrap endpoint.
type BootstrapOut struct {
	Body BootstrapBody
}
