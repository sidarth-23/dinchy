// Package auth handles authentication, sessions, account setup, and related feature flows.
package auth

import (
	"context"
	"errors"
	"net/http"

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
		Errors:      []int{http.StatusForbidden},
	}, a.bootstrap)

	huma.Register(h, huma.Operation{
		OperationID: "auth-login",
		Method:      http.MethodPost,
		Path:        "/auth/login",
		Summary:     "Authenticate with email and password",
		Description: "Validates credentials and issues a session cookie. Returns the current bootstrap state with viewer info.",
		Tags:        []string{"Auth"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, a.login)

	huma.Register(h, huma.Operation{
		OperationID: "auth-logout",
		Method:      http.MethodPost,
		Path:        "/auth/logout",
		Summary:     "End the current session",
		Description: "Revokes the current session and clears the session cookie.",
		Tags:        []string{"Auth"},
		Errors:      []int{http.StatusForbidden},
	}, a.logout)

	huma.Register(h, huma.Operation{
		OperationID: "auth-session",
		Method:      http.MethodGet,
		Path:        "/auth/session",
		Summary:     "Get current session state",
		Description: "Returns bootstrap state for the current request. Used to validate that a session is still active.",
		Tags:        []string{"Auth"},
		Errors:      []int{http.StatusForbidden},
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
		Errors:      []int{http.StatusBadRequest, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, a.ssoStart)

	huma.Register(h, huma.Operation{
		OperationID: "auth-sso-callback",
		Method:      http.MethodGet,
		Path:        "/auth/sso/{provider_id}/callback",
		Summary:     "Complete an SSO login",
		Description: "Consumes the provider callback, creates a session, and redirects back into the app.",
		Tags:        []string{"Auth", "SSO"},
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, a.ssoCallback)

	huma.Register(h, huma.Operation{
		OperationID: "auth-select-organization",
		Method:      http.MethodPost,
		Path:        "/auth/organizations/select",
		Summary:     "Switch active organization",
		Description: "Sets the active organization for the current session and reissues the session cookie.",
		Tags:        []string{"Auth"},
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, a.selectOrganisation)

	huma.Register(h, huma.Operation{
		OperationID: "auth-create-invitation",
		Method:      http.MethodPost,
		Path:        "/auth/invitations",
		Summary:     "Create an organization invitation",
		Description: "Invites a member to the current organization. Requires an owner or admin session.",
		Tags:        []string{"Auth"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity},
	}, a.createInvitation)

	huma.Register(h, huma.Operation{
		OperationID: "auth-accept-invitation",
		Method:      http.MethodPost,
		Path:        "/auth/invitations/{token}/accept",
		Summary:     "Accept an organization invitation",
		Description: "Consumes an invitation token, provisions the member account, and issues a session cookie.",
		Tags:        []string{"Auth"},
		Errors:      []int{http.StatusBadRequest, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, a.acceptInvitation)

	huma.Register(h, huma.Operation{
		OperationID: "auth-forgot-password",
		Method:      http.MethodPost,
		Path:        "/auth/forgot-password",
		Summary:     "Request a password reset",
		Description: "Sends a password reset email when the address matches an account. Always returns 200 to avoid account enumeration.",
		Tags:        []string{"Auth"},
		Errors:      []int{http.StatusUnprocessableEntity},
	}, a.forgotPassword)

	huma.Register(h, huma.Operation{
		OperationID: "auth-reset-password",
		Method:      http.MethodPost,
		Path:        "/auth/reset-password",
		Summary:     "Reset password",
		Description: "Sets a new password using a valid reset token and revokes existing sessions.",
		Tags:        []string{"Auth"},
		Errors:      []int{http.StatusBadRequest, http.StatusUnprocessableEntity},
	}, a.resetPassword)

	huma.Register(h, huma.Operation{
		OperationID: "auth-totp-enroll",
		Method:      http.MethodPost,
		Path:        "/auth/totp/enroll",
		Summary:     "Start TOTP enrollment",
		Description: "Generates a TOTP secret and provisioning URL for the current user.",
		Tags:        []string{"Auth", "TOTP"},
		Errors:      []int{http.StatusUnauthorized},
	}, a.totpEnroll)

	huma.Register(h, huma.Operation{
		OperationID: "auth-totp-confirm",
		Method:      http.MethodPost,
		Path:        "/auth/totp/confirm",
		Summary:     "Confirm TOTP enrollment",
		Description: "Verifies a TOTP code and enables two-factor authentication for the current user.",
		Tags:        []string{"Auth", "TOTP"},
		Errors:      []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusTooManyRequests},
	}, a.totpConfirm)

	huma.Register(h, huma.Operation{
		OperationID: "auth-totp-disable",
		Method:      http.MethodPost,
		Path:        "/auth/totp/disable",
		Summary:     "Disable TOTP",
		Description: "Disables two-factor authentication for the current user.",
		Tags:        []string{"Auth", "TOTP"},
		Errors:      []int{http.StatusUnauthorized},
	}, a.totpDisable)

	huma.Register(h, huma.Operation{
		OperationID: "setup-first-user",
		Method:      http.MethodPost,
		Path:        "/setup/first-user",
		Summary:     "Create the first admin user",
		Description: "Creates the initial admin account. Returns 409 if setup has already been completed.",
		Tags:        []string{"Setup"},
		Errors:      []int{http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity},
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
	providers, err := a.auth.listSSOProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := &SSOProvidersOut{Body: providers}
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

func (a *API) createInvitation(ctx context.Context, in *CreateInvitationIn) (*CreateInvitationOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	sess := SessionFrom(ctx)
	if sess == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	if sess.Role != RoleOwner && sess.Role != RoleAdmin {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthForbidden))
	}
	invitation, err := a.auth.CreateInvitation(ctx, sess, in.Body.Email, in.Body.Role, support.RemoteIPFrom(ctx), support.UserAgentFrom(ctx))
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthInvitationCreate),
			apperrors.WithStage(apperrors.StageCreateInvitation),
		)
	}
	out := &CreateInvitationOut{}
	out.Body.Created = invitation.ID != ""
	return out, nil
}

func (a *API) acceptInvitation(ctx context.Context, in *AcceptInvitationIn) (*AcceptInvitationOut, error) {
	if a.requireHTTPS && !support.IsSecure(ctx) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired))
	}
	token, err := a.auth.AcceptInvitation(
		ctx,
		in.Token,
		in.Body.DisplayName,
		in.Body.Password,
		support.RemoteIPFrom(ctx),
		support.UserAgentFrom(ctx),
	)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthInvitationAccept),
			apperrors.WithStage(apperrors.StageAcceptInvitation),
		)
	}
	bs, err := a.settings.Bootstrap(ctx)
	if err != nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthInvitationAccept),
			apperrors.WithStage(apperrors.StageBootstrap),
		)
	}
	sess, err := a.auth.Session(ctx, token)
	if err != nil || sess == nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithHandler(apperrors.HandlerAuthInvitationAccept),
			apperrors.WithStage(apperrors.StageSessionLookup),
		)
	}
	secure := support.IsSecure(ctx)
	out := &AcceptInvitationOut{}
	out.SetCookie = []http.Cookie{*a.auth.SessionCookie(token, secure)}
	out.Body.SetupRequired = false
	out.Body.Authenticated = true
	out.Body.App.InstanceName = bs.InstanceName
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
		in.Body.Email,
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
	body.Organizations = organisationsOut(orgs)
	return nil
}

func organisationsOut(orgs []Organization) []OrganisationOut {
	out := make([]OrganisationOut, 0, len(orgs))
	for _, org := range orgs {
		out = append(out, OrganisationOut{ID: org.ID, Name: org.Name, Slug: org.Slug, Role: string(org.Role)})
	}
	return out
}
