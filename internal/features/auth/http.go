package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/bootstrap"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// LoginBody contains the credentials required to authenticate.
type LoginBody struct {
	Email    string `json:"email" format:"email" minLength:"3" maxLength:"254" doc:"User email address"`
	Password string `json:"password" minLength:"1" maxLength:"128" doc:"User password"`
}

// LoginIn is the huma input type for the login endpoint.
type LoginIn struct {
	Body LoginBody
}

// LoginOut returns the bootstrap state and sets the session cookie on success.
type LoginOut struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      bootstrap.BootstrapBody
}

// LogoutIn reads the session cookie so the handler can revoke it.
type LogoutIn struct {
	DinchySession string `cookie:"dinchy_session"`
}

// LogoutOut clears the session cookie.
type LogoutOut struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
}

// SessionOut returns the current bootstrap state (same shape as bootstrap).
type SessionOut struct {
	Body bootstrap.BootstrapBody
}

// SetupBody contains the fields required to create the first admin user.
type SetupBody struct {
	Email       string `json:"email" format:"email" minLength:"3" maxLength:"254" doc:"Admin email address"`
	DisplayName string `json:"display_name" minLength:"1" maxLength:"100" doc:"Display name for the admin user"`
	Password    string `json:"password" minLength:"8" maxLength:"128" doc:"Password (minimum 8 characters)"`
}

// SetupIn is the huma input type for the first-user setup endpoint.
type SetupIn struct {
	Body SetupBody
}

// SetupOut returns the bootstrap state and sets the session cookie on success.
type SetupOut struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      bootstrap.BootstrapBody
}

// API groups the auth handlers and their shared dependencies.
type API struct {
	auth         *Service
	settings     bootstrap.SettingsReader
	requireHTTPS bool
}

// Register mounts the auth operations on the given huma.API instance.
func Register(h huma.API, svc *Service, sr bootstrap.SettingsReader, requireHTTPS bool) {
	a := &API{auth: svc, settings: sr, requireHTTPS: requireHTTPS}

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
			apperrors.WithMeta("handler", "auth.login"),
			apperrors.WithMeta("stage", "login"),
		)
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithMeta("handler", "auth.login"),
			apperrors.WithMeta("stage", "bootstrap"),
		)
	}
	sess, err := a.auth.Session(ctx, token)
	if err != nil || sess == nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithMeta("handler", "auth.login"),
			apperrors.WithMeta("stage", "session_lookup"),
		)
	}
	secure := support.IsSecure(ctx)
	out := &LoginOut{}
	out.SetCookie = []http.Cookie{*support.SessionCookie(token, secure)}
	out.Body.SetupRequired = false
	out.Body.Authenticated = true
	out.Body.App.InstanceName = bs.InstanceName
	out.Body.Viewer = &bootstrap.ViewerOut{
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
				apperrors.WithMeta("handler", "auth.logout"),
				apperrors.WithMeta("stage", "logout"),
			)
		}
	}
	out := &LogoutOut{}
	out.SetCookie = *support.ClearSessionCookie(support.IsSecure(ctx))
	return out, nil
}

func (a *API) session(ctx context.Context, _ *struct{}) (*SessionOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithMeta("handler", "auth.session"),
			apperrors.WithMeta("stage", "bootstrap"),
		)
	}
	out := &SessionOut{}
	out.Body.SetupRequired = bs.SetupRequired
	out.Body.App.InstanceName = bs.InstanceName
	if sess := support.SessionFrom(ctx); sess != nil {
		out.Body.Authenticated = true
		out.Body.Viewer = &bootstrap.ViewerOut{
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
			apperrors.WithMeta("handler", "auth.setup"),
			apperrors.WithMeta("stage", "setup_first_user"),
		)
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithMeta("handler", "auth.setup"),
			apperrors.WithMeta("stage", "bootstrap"),
		)
	}
	sess, err := a.auth.Session(ctx, token)
	if err != nil || sess == nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithMeta("handler", "auth.setup"),
			apperrors.WithMeta("stage", "session_lookup"),
		)
	}
	secure := support.IsSecure(ctx)
	out := &SetupOut{}
	out.SetCookie = []http.Cookie{*support.SessionCookie(token, secure)}
	out.Body.SetupRequired = false
	out.Body.Authenticated = true
	out.Body.App.InstanceName = bs.InstanceName
	out.Body.Viewer = &bootstrap.ViewerOut{
		Email:       sess.Email,
		DisplayName: sess.DisplayName,
		Role:        string(sess.Role),
	}
	return out, nil
}
