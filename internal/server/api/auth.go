package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/sidarth-23/dinchy/internal/server/apierr"
	"github.com/sidarth-23/dinchy/internal/server/support"
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
	Body      BootstrapBody
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
	Body BootstrapBody
}

func (a *API) registerAuth(h huma.API) {
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
}

func (a *API) login(ctx context.Context, in *LoginIn) (*LoginOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apierr.Localized(ctx, apierr.ErrHTTPSRequired())
	}
	token, err := a.auth.Login(
		ctx,
		in.Body.Email,
		in.Body.Password,
		support.RemoteIPFrom(ctx),
		support.UserAgentFrom(ctx),
	)
	if err != nil {
		return nil, apierr.MapServiceError(ctx, err)
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apierr.Localized(ctx, apierr.ErrInternal())
	}
	sess, err := a.auth.Session(ctx, token)
	if err != nil || sess == nil {
		return nil, apierr.Localized(ctx, apierr.ErrInternal())
	}
	secure := support.IsSecure(ctx)
	out := &LoginOut{}
	out.SetCookie = []http.Cookie{*support.SessionCookie(token, secure)}
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
		return nil, apierr.Localized(ctx, apierr.ErrHTTPSRequired())
	}
	if in.DinchySession != "" {
		if err := a.auth.Logout(ctx, in.DinchySession); err != nil {
			return nil, apierr.Localized(ctx, apierr.ErrInternal())
		}
	}
	out := &LogoutOut{}
	out.SetCookie = *support.ClearSessionCookie(support.IsSecure(ctx))
	return out, nil
}

func (a *API) session(ctx context.Context, _ *struct{}) (*SessionOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apierr.Localized(ctx, apierr.ErrHTTPSRequired())
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apierr.Localized(ctx, apierr.ErrInternal())
	}
	out := &SessionOut{}
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
