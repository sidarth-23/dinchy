// Package auth handles password hashing, session issuance, and session validation.
package auth

import (
	"context"
	"errors"
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
		OperationID: "auth-sso-providers",
		Method:      http.MethodGet,
		Path:        "/auth/sso/providers",
		Summary:     "List configured SSO providers",
		Description: "Returns the configured SSO providers that can be used for provider login.",
		Tags:        []string{"Auth", "SSO"},
	}, a.ssoProviders)

	huma.Register(h, huma.Operation{
		OperationID: "auth-sso-start",
		Method:      http.MethodGet,
		Path:        "/auth/sso/{provider_id}/start",
		Summary:     "Start an SSO login",
		Description: "Redirects to the provider authorization URL and sets a short-lived state cookie.",
		Tags:        []string{"Auth", "SSO"},
	}, a.ssoStart)

	huma.Register(h, huma.Operation{
		OperationID: "auth-sso-callback",
		Method:      http.MethodGet,
		Path:        "/auth/sso/{provider_id}/callback",
		Summary:     "Complete an SSO login",
		Description: "Consumes the provider callback, creates a session, and redirects back into the app.",
		Tags:        []string{"Auth", "SSO"},
	}, a.ssoCallback)

	huma.Register(h, huma.Operation{
		OperationID: "auth-select-organisation",
		Method:      http.MethodPost,
		Path:        "/auth/organisations/select",
		Summary:     "Switch active organisation",
		Tags:        []string{"Auth"},
	}, a.selectOrganisation)

	huma.Register(h, huma.Operation{
		OperationID: "auth-forgot-password",
		Method:      http.MethodPost,
		Path:        "/auth/forgot-password",
		Summary:     "Request a password reset",
		Tags:        []string{"Auth"},
	}, a.forgotPassword)

	huma.Register(h, huma.Operation{
		OperationID: "auth-reset-password",
		Method:      http.MethodPost,
		Path:        "/auth/reset-password",
		Summary:     "Reset password",
		Tags:        []string{"Auth"},
	}, a.resetPassword)

	huma.Register(h, huma.Operation{
		OperationID: "auth-totp-enroll",
		Method:      http.MethodPost,
		Path:        "/auth/totp/enroll",
		Summary:     "Start TOTP enrollment",
		Tags:        []string{"Auth", "TOTP"},
	}, a.totpEnroll)

	huma.Register(h, huma.Operation{
		OperationID: "auth-totp-confirm",
		Method:      http.MethodPost,
		Path:        "/auth/totp/confirm",
		Summary:     "Confirm TOTP enrollment",
		Tags:        []string{"Auth", "TOTP"},
	}, a.totpConfirm)

	huma.Register(h, huma.Operation{
		OperationID: "auth-totp-disable",
		Method:      http.MethodPost,
		Path:        "/auth/totp/disable",
		Summary:     "Disable TOTP",
		Tags:        []string{"Auth", "TOTP"},
	}, a.totpDisable)

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
	if err := a.attachSession(ctx, &out.Body); err != nil {
		return nil, err
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
		in.Body.OrganisationSlug,
		in.Body.TOTPCode,
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
	out.SetCookie = []http.Cookie{*a.auth.SessionCookie(token, secure)}
	out.Body.SetupRequired = false
	out.Body.Authenticated = true
	out.Body.App.InstanceName = bs.InstanceName
	if err := a.populateAuthenticatedBody(ctx, &out.Body, sess, bs.InstanceName); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *API) logout(ctx context.Context, in *LogoutIn) (*LogoutOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	sessionToken := support.CookieValueFrom(ctx, a.auth.SessionCookieName())
	if sessionToken != "" {
		if err := a.auth.Logout(ctx, sessionToken); err != nil {
			return nil, apperrors.Annotate(err,
				apperrors.WithHandler(apperrors.HandlerAuthLogout),
				apperrors.WithStage(apperrors.StageLogout),
			)
		}
	}
	out := &LogoutOut{}
	out.SetCookie = *a.auth.ClearSessionCookie(support.IsSecure(ctx))
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
	if err := a.attachSession(ctx, &out.Body); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *API) ssoProviders(ctx context.Context, _ *struct{}) (*SSOProvidersOut, error) {
	out := &SSOProvidersOut{Body: a.auth.listSSOProviders()}
	return out, nil
}

func (a *API) ssoStart(ctx context.Context, in *SSOStartIn) (*SSOStartOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	authURL, cookies, err := a.auth.startSSO(ctx, in.ProviderID, in.ReturnTo, in.OrganisationSlug)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthSSOStart),
			apperrors.WithStage(apperrors.StageSSOStart),
		)
	}
	secure := support.IsSecure(ctx)
	for i := range cookies {
		cookies[i].Secure = secure
	}
	return &SSOStartOut{Status: http.StatusFound, Location: authURL, SetCookie: cookies}, nil
}

func (a *API) ssoCallback(ctx context.Context, in *SSOCallbackIn) (*SSOCallbackOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	if in.Error != "" {
		return nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOLoginFailed), apperrors.WithCause(errors.New(in.ErrorDetail)))
	}
	returnTo, token, clearCookie, err := a.auth.completeSSO(
		ctx,
		in.ProviderID,
		in.State,
		in.Code,
		support.CookieValueFrom(ctx, a.auth.SSOStateCookieName()),
		support.CookieValueFrom(ctx, a.auth.SSOSessionCookieName()),
		support.RemoteIPFrom(ctx),
		support.UserAgentFrom(ctx),
	)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthSSOCallback),
			apperrors.WithStage(apperrors.StageSSOCallback),
		)
	}
	secure := support.IsSecure(ctx)
	for i := range clearCookie {
		clearCookie[i].Secure = secure
	}
	out := &SSOCallbackOut{
		Status:    http.StatusFound,
		Location:  returnTo,
		SetCookie: append([]http.Cookie{*a.auth.SessionCookie(token, secure)}, clearCookie...),
	}
	return out, nil
}

func (a *API) selectOrganisation(ctx context.Context, in *SelectOrganisationIn) (*SelectOrganisationOut, error) {
	token, err := a.auth.SelectOrganisation(ctx, support.CookieValueFrom(ctx, a.auth.SessionCookieName()), in.Body.OrganisationSlug, support.RemoteIPFrom(ctx), support.UserAgentFrom(ctx))
	if err != nil {
		return nil, err
	}
	sess, err := a.auth.Session(ctx, token)
	if err != nil || sess == nil {
		return nil, apperrors.Annotate(err, apperrors.WithHandler(apperrors.HandlerAuthSession), apperrors.WithStage(apperrors.StageSessionLookup))
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	out := &SelectOrganisationOut{SetCookie: []http.Cookie{*a.auth.SessionCookie(token, support.IsSecure(ctx))}}
	if err := a.populateAuthenticatedBody(ctx, &out.Body, sess, bs.InstanceName); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *API) forgotPassword(ctx context.Context, in *ForgotPasswordIn) (*ForgotPasswordOut, error) {
	if err := a.auth.ForgotPassword(ctx, in.Body.Email); err != nil {
		return nil, err
	}
	out := &ForgotPasswordOut{}
	out.Body.Accepted = true
	return out, nil
}

func (a *API) resetPassword(ctx context.Context, in *ResetPasswordIn) (*ResetPasswordOut, error) {
	if err := a.auth.ResetPassword(ctx, in.Body.Token, in.Body.Password); err != nil {
		return nil, err
	}
	out := &ResetPasswordOut{}
	out.Body.Reset = true
	return out, nil
}

func (a *API) totpEnroll(ctx context.Context, _ *struct{}) (*TOTPEnrollOut, error) {
	sess := SessionFrom(ctx)
	if sess == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	secret, url, err := a.auth.StartTOTPEnrollment(ctx, sess.UserID, sess.Email)
	if err != nil {
		return nil, err
	}
	out := &TOTPEnrollOut{}
	out.Body.Secret = secret
	out.Body.URL = url
	return out, nil
}

func (a *API) totpConfirm(ctx context.Context, in *TOTPConfirmIn) (*TOTPConfirmOut, error) {
	sess := SessionFrom(ctx)
	if sess == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	if err := a.auth.ConfirmTOTP(ctx, sess.UserID, in.Body.Code); err != nil {
		return nil, err
	}
	out := &TOTPConfirmOut{}
	out.Body.Enabled = true
	return out, nil
}

func (a *API) totpDisable(ctx context.Context, _ *struct{}) (*TOTPConfirmOut, error) {
	sess := SessionFrom(ctx)
	if sess == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	if err := a.auth.DisableTOTP(ctx, sess.UserID); err != nil {
		return nil, err
	}
	out := &TOTPConfirmOut{}
	out.Body.Enabled = false
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
	out.SetCookie = []http.Cookie{*a.auth.SessionCookie(token, secure)}
	out.Body.SetupRequired = false
	out.Body.Authenticated = true
	out.Body.App.InstanceName = bs.InstanceName
	if err := a.populateAuthenticatedBody(ctx, &out.Body, sess, bs.InstanceName); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *API) attachSession(ctx context.Context, body *BootstrapBody) error {
	sess := SessionFrom(ctx)
	if sess == nil {
		return nil
	}
	return a.populateAuthenticatedBody(ctx, body, sess, body.App.InstanceName)
}

func (a *API) populateAuthenticatedBody(ctx context.Context, body *BootstrapBody, sess *SessionWithUser, instanceName string) error {
	orgs, err := a.auth.OrganisationsForUser(ctx, sess.UserID)
	if err != nil {
		return err
	}
	body.SetupRequired = false
	body.Authenticated = true
	body.App.InstanceName = instanceName
	body.Viewer = &ViewerOut{Email: sess.Email, DisplayName: sess.DisplayName, Role: string(sess.Role)}
	body.ActiveOrganisation = &OrganisationOut{ID: sess.OrganisationID, Name: sess.OrganisationName, Slug: sess.OrganisationSlug, Role: string(sess.Role)}
	body.Organisations = organisationsOut(orgs)
	return nil
}

func organisationsOut(orgs []Organisation) []OrganisationOut {
	out := make([]OrganisationOut, 0, len(orgs))
	for _, org := range orgs {
		out = append(out, OrganisationOut{ID: org.ID, Name: org.Name, Slug: org.Slug, Role: string(org.Role)})
	}
	return out
}
