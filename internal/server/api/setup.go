package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/sidarth-23/dinchy/internal/server/support"
)

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
	Body      BootstrapBody
}

func (a *API) registerSetup(h huma.API) {
	huma.Register(h, huma.Operation{
		OperationID: "setup-first-user",
		Method:      http.MethodPost,
		Path:        "/setup/first-user",
		Summary:     "Create the first admin user",
		Description: "Creates the initial admin account. Returns 409 if setup has already been completed.",
		Tags:        []string{"Setup"},
	}, a.setup)
}

func (a *API) setup(ctx context.Context, in *SetupIn) (*SetupOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, ErrHTTPSRequired()
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
		return nil, MapServiceError(err)
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, ErrInternal()
	}
	sess, err := a.auth.Session(ctx, token)
	if err != nil || sess == nil {
		return nil, ErrInternal()
	}
	secure := support.IsSecure(ctx)
	out := &SetupOut{}
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
