package bootstrap

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

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

// API groups the bootstrap handler and its shared dependencies.
type API struct {
	settings     SettingsReader
	requireHTTPS bool
}

// Register mounts the bootstrap operation on the given huma.API instance.
func Register(h huma.API, sr SettingsReader, requireHTTPS bool) {
	a := &API{settings: sr, requireHTTPS: requireHTTPS}
	huma.Register(h, huma.Operation{
		OperationID: "get-bootstrap",
		Method:      http.MethodGet,
		Path:        "/bootstrap",
		Summary:     "Get application bootstrap state",
		Description: "Returns setup status, authentication state, app metadata, and current user info. Called by the frontend on initial load.",
		Tags:        []string{"Bootstrap"},
	}, a.bootstrap)
}

func (a *API) bootstrap(ctx context.Context, _ *struct{}) (*BootstrapOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerBootstrapGet),
			apperrors.WithStage(apperrors.StageBootstrap),
		)
	}
	out := &BootstrapOut{}
	out.Body.SetupRequired = bs.SetupRequired
	out.Body.App.InstanceName = bs.InstanceName
	if sess := support.SessionFrom(ctx); sess != nil {
		out.Body.Authenticated = true
		out.Body.Viewer = &ViewerOut{
			Email:       sess.Email,
			DisplayName: sess.DisplayName,
			Role:        string(sess.Role),
		}
	}
	return out, nil
}
