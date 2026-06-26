// Package auth handles password hashing, session issuance, and session validation.
package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// API groups the auth handlers and their shared dependencies.
type API struct {
	auth         *Service
	settings     SettingsReader
	requireHTTPS bool
}

// Register mounts the auth operations on the given huma.API instance.
func Register(h huma.API, svc *Service, sr SettingsReader, requireHTTPS bool) {
	a := &API{auth: svc, settings: sr, requireHTTPS: requireHTTPS}

	huma.Register(h, huma.Operation{
		OperationID: "get-bootstrap",
		Method:      http.MethodGet,
		Path:        "/bootstrap",
		Summary:     "Get application bootstrap state",
		Description: "Returns setup status, authentication state, app metadata, and current user info. Called by the frontend on initial load.",
		Tags:        []string{"Bootstrap"},
	}, a.bootstrap)

	huma.Register(h, huma.Operation{
		OperationID: "auth-login",
		Method:      http.MethodPost,
		Path:        "/auth/login",
		Summary:     "Authenticate with email and password",
		Description: "Validates credentials and issues a session cookie. Returns the current bootstrap state with viewer info.",
		Tags:        []string{"Auth"},
	}, a.login)

	huma.Register(h, huma.Operation{
		OperationID: "auth-logout",
		Method:      http.MethodPost,
		Path:        "/auth/logout",
		Summary:     "End the current session",
		Description: "Revokes the current session and clears the session cookie.",
		Tags:        []string{"Auth"},
	}, a.logout)

	huma.Register(h, huma.Operation{
		OperationID: "auth-session",
		Method:      http.MethodGet,
		Path:        "/auth/session",
		Summary:     "Get current session state",
		Description: "Returns bootstrap state for the current request. Used to validate that a session is still active.",
		Tags:        []string{"Auth"},
	}, a.session)

	huma.Register(h, huma.Operation{
		OperationID: "setup-first-user",
		Method:      http.MethodPost,
		Path:        "/setup/first-user",
		Summary:     "Create the first admin user",
		Description: "Creates the initial admin account. Returns 409 if setup has already been completed.",
		Tags:        []string{"Setup"},
	}, a.setup)
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
	if sess := SessionFrom(ctx); sess != nil {
		out.Body.Authenticated = true
		out.Body.Viewer = &ViewerOut{
			Email:       sess.Email,
			DisplayName: sess.DisplayName,
			Role:        string(sess.Role),
		}
	}
	return out, nil
}

func (a *API) login(ctx context.Context, in *LoginIn) (*LoginOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	token, err := a.auth.Login(
		ctx,
		in.Body.Email,
		in.Body.Password,
		support.RemoteIPFrom(ctx),
		support.UserAgentFrom(ctx),
	)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthLogin),
			apperrors.WithStage(apperrors.StageLogin),
		)
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthLogin),
			apperrors.WithStage(apperrors.StageBootstrap),
		)
	}
	sess, err := a.auth.Session(ctx, token)
	if err != nil || sess == nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthLogin),
			apperrors.WithStage(apperrors.StageSessionLookup),
		)
	}
	secure := support.IsSecure(ctx)
	out := &LoginOut{}
	out.SetCookie = []http.Cookie{*SessionCookie(token, secure)}
	out.Body.SetupRequired = false
	out.Body.Authenticated = true
	out.Body.App.InstanceName = bs.InstanceName
	out.Body.Viewer = &ViewerOut{
		Email:       sess.Email,
		DisplayName: sess.DisplayName,
		Role:        string(sess.Role),
	}
	return out, nil
}

func (a *API) logout(ctx context.Context, in *LogoutIn) (*LogoutOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	if in.DinchySession != "" {
		if err := a.auth.Logout(ctx, in.DinchySession); err != nil {
			return nil, apperrors.Annotate(err,
				apperrors.WithHandler(apperrors.HandlerAuthLogout),
				apperrors.WithStage(apperrors.StageLogout),
			)
		}
	}
	out := &LogoutOut{}
	out.SetCookie = *ClearSessionCookie(support.IsSecure(ctx))
	return out, nil
}

func (a *API) session(ctx context.Context, _ *struct{}) (*SessionOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthSession),
			apperrors.WithStage(apperrors.StageBootstrap),
		)
	}
	out := &SessionOut{}
	out.Body.SetupRequired = bs.SetupRequired
	out.Body.App.InstanceName = bs.InstanceName
	if sess := SessionFrom(ctx); sess != nil {
		out.Body.Authenticated = true
		out.Body.Viewer = &ViewerOut{
			Email:       sess.Email,
			DisplayName: sess.DisplayName,
			Role:        string(sess.Role),
		}
	}
	return out, nil
}

func (a *API) setup(ctx context.Context, in *SetupIn) (*SetupOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	token, err := a.auth.SetupFirstUser(
		ctx,
		strings.ToLower(in.Body.Email),
		in.Body.DisplayName,
		in.Body.Password,
		support.RemoteIPFrom(ctx),
		support.UserAgentFrom(ctx),
	)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthSetup),
			apperrors.WithStage(apperrors.StageSetupFirstUser),
		)
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthSetup),
			apperrors.WithStage(apperrors.StageBootstrap),
		)
	}
	sess, err := a.auth.Session(ctx, token)
	if err != nil || sess == nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthSetup),
			apperrors.WithStage(apperrors.StageSessionLookup),
		)
	}
	secure := support.IsSecure(ctx)
	out := &SetupOut{}
	out.SetCookie = []http.Cookie{*SessionCookie(token, secure)}
	out.Body.SetupRequired = false
	out.Body.Authenticated = true
	out.Body.App.InstanceName = bs.InstanceName
	out.Body.Viewer = &ViewerOut{
		Email:       sess.Email,
		DisplayName: sess.DisplayName,
		Role:        string(sess.Role),
	}
	return out, nil
}
