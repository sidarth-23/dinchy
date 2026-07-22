package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

//go:generate mockgen -self_package=github.com/sidarth-23/dinchy/internal/features/auth -destination=store_mockdata_test.go -package=auth . Store

// AccountProvider identifies the authentication provider backing an account.
type AccountProvider string

// Supported account providers.
const (
	AccountProviderPassword AccountProvider = "password"
)

// User is an authenticated principal.
type User struct {
	ID            string
	Email         string
	DisplayName   string
	EmailVerified bool
	Disabled      bool
}

// Organization describes a user-visible organization and the caller's role in it.
type Organization struct {
	ID   string
	Name string
	Slug string
	Role permission.Role
}

// InvitationStatus is the lifecycle state of an organization invitation.
type InvitationStatus string

// Recognized invitation statuses.
const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusRejected InvitationStatus = "rejected"
	InvitationStatusCanceled InvitationStatus = "canceled"
)

// Invitation is a pending or resolved invitation for a user to join an organization.
type Invitation struct {
	ID              string
	OrganizationID  string
	Email           string
	Role            permission.Role
	Status          InvitationStatus
	ExpiresAt       time.Time
	InvitedByUserID string
	AcceptedAt      time.Time
	AcceptedAtValid bool
}

// TwoFactor is a user's TOTP two-factor enrollment and its verification state.
type TwoFactor struct {
	ID                      string
	UserID                  string
	Secret                  string
	Verified                bool
	LastUsedStep            int64
	LastUsedStepValid       bool
	FailedVerificationCount int64
	LockedUntil             time.Time
	LockedUntilValid        bool
}

// CreateUserInput carries everything needed to provision a user together with their
// first account, organization, and membership.
type CreateUserInput struct {
	ID                   string
	AccountID            string
	OrganizationID       string
	OrganizationMemberID string
	AdminRoleID          string
	MemberRoleID         string
	Email                string
	PasswordHash         string
	DisplayName          string
	OrganizationName     string
	OrganizationSlug     string
	Now                  time.Time
}

// Store is the persistence surface the auth feature depends on.
type Store interface {
	CountUsers(ctx context.Context) (int64, error)
	InsertUser(ctx context.Context, arg sqlcgen.InsertUserParams) error
	InsertAccount(ctx context.Context, arg sqlcgen.InsertAccountParams) error
	InsertOrganization(ctx context.Context, arg sqlcgen.InsertOrganizationParams) error
	InsertOrganizationRole(ctx context.Context, arg sqlcgen.InsertOrganizationRoleParams) error
	InsertOrganizationRolePermission(ctx context.Context, arg sqlcgen.InsertOrganizationRolePermissionParams) error
	InsertOrganizationMember(ctx context.Context, arg sqlcgen.InsertOrganizationMemberParams) error
	FindUserByEmail(ctx context.Context, email string) (sqlcgen.FindUserByEmailRow, error)
	UpdateUserEmailVerifiedAt(ctx context.Context, arg sqlcgen.UpdateUserEmailVerifiedAtParams) error
	FindPasswordAccountByUserID(ctx context.Context, userID uuid.UUID) (sqlcgen.FindPasswordAccountByUserIDRow, error)
	FindUserByProviderAccount(ctx context.Context, arg sqlcgen.FindUserByProviderAccountParams) (sqlcgen.FindUserByProviderAccountRow, error)
	ListOrganizationsForUser(ctx context.Context, userID uuid.UUID) ([]sqlcgen.ListOrganizationsForUserRow, error)
	FindOrganizationBySlugForUser(ctx context.Context, arg sqlcgen.FindOrganizationBySlugForUserParams) (sqlcgen.FindOrganizationBySlugForUserRow, error)
	FindOrganizationByIDForUser(ctx context.Context, arg sqlcgen.FindOrganizationByIDForUserParams) (sqlcgen.FindOrganizationByIDForUserRow, error)
	UpdateUserPasswordHash(ctx context.Context, arg sqlcgen.UpdateUserPasswordHashParams) error
	InsertVerificationToken(ctx context.Context, arg sqlcgen.InsertVerificationTokenParams) error
	FindVerificationToken(ctx context.Context, arg sqlcgen.FindVerificationTokenParams) (sqlcgen.FindVerificationTokenRow, error)
	ConsumeVerificationToken(ctx context.Context, arg sqlcgen.ConsumeVerificationTokenParams) error
	InsertOrganizationInvitation(ctx context.Context, arg sqlcgen.InsertOrganizationInvitationParams) error
	FindOrganizationInvitationByToken(ctx context.Context, tokenHash string) (sqlcgen.FindOrganizationInvitationByTokenRow, error)
	FindPendingOrganizationInvitationByEmail(ctx context.Context, arg sqlcgen.FindPendingOrganizationInvitationByEmailParams) (sqlcgen.FindPendingOrganizationInvitationByEmailRow, error)
	ConsumeOrganizationInvitation(ctx context.Context, arg sqlcgen.ConsumeOrganizationInvitationParams) error
	InsertOrReplaceTwoFactor(ctx context.Context, arg sqlcgen.InsertOrReplaceTwoFactorParams) error
	FindTwoFactorByUserID(ctx context.Context, userID uuid.UUID) (sqlcgen.FindTwoFactorByUserIDRow, error)
	ConfirmTwoFactor(ctx context.Context, arg sqlcgen.ConfirmTwoFactorParams) error
	MarkTwoFactorUsed(ctx context.Context, arg sqlcgen.MarkTwoFactorUsedParams) error
	RegisterTwoFactorFailure(ctx context.Context, arg sqlcgen.RegisterTwoFactorFailureParams) error
	DisableTwoFactor(ctx context.Context, userID uuid.UUID) error
	GetInstanceName(ctx context.Context) (string, error)
}

// BootstrapState reports whether first-user setup is required and the instance name.
type BootstrapState struct {
	SetupRequired bool
	InstanceName  string
}

// SettingsReader exposes the instance settings the auth feature reads at bootstrap.
type SettingsReader interface {
	Bootstrap(ctx context.Context) (BootstrapState, error)
}

// ViewerOut is the current authenticated user projection returned in bootstrap responses.
type ViewerOut struct {
	Email       string `json:"email" doc:"User email address"`
	DisplayName string `json:"display_name" doc:"User display name"`
	Role        string `json:"role" doc:"User role"`
}

// OrganizationOut is an organization projection returned in bootstrap responses.
type OrganizationOut struct {
	ID   string `json:"id" doc:"Organization identifier"`
	Name string `json:"name" doc:"Organization display name"`
	Slug string `json:"slug" doc:"Organization slug"`
	Role string `json:"role" doc:"Viewer's role in this organization"`
}

// AppOut contains application-level metadata returned in every API response body.
type AppOut struct {
	InstanceName string `json:"instance_name" doc:"Name of this Dinchy instance"`
}

// BootstrapBody is the shared response body for bootstrap, session, login, and setup endpoints.
type BootstrapBody struct {
	SetupRequired      bool              `json:"setup_required" doc:"True when no users exist and first-user setup must be completed"`
	Authenticated      bool              `json:"authenticated" doc:"True when the request carries a valid session cookie"`
	App                AppOut            `json:"app" doc:"Application-level metadata"`
	Viewer             *ViewerOut        `json:"viewer" doc:"Current authenticated user, or null when not authenticated"`
	ActiveOrganization *OrganizationOut  `json:"active_organization,omitempty"`
	Organizations      []OrganizationOut `json:"organizations,omitempty"`
}

// BootstrapOut is the response type for the bootstrap endpoint.
type BootstrapOut struct {
	Body BootstrapBody
}

// LoginBody contains the credentials required to authenticate.
type LoginBody struct {
	Email            string `json:"email" format:"email" minLength:"3" maxLength:"254" example:"user@example.com" doc:"User email address"`
	Password         string `json:"password" minLength:"1" maxLength:"128" example:"correct horse battery staple" doc:"User password"`
	OrganizationSlug string `json:"organization_slug,omitempty" maxLength:"64" example:"acme" doc:"Organization slug when the user has multiple memberships"`
	TOTPCode         string `json:"totp_code,omitempty" maxLength:"8" example:"123456" doc:"TOTP code when two-factor authentication is enabled"`
}

// Resolve normalizes the login credentials before validation errors are returned
// so downstream services receive canonical values.
func (b *LoginBody) Resolve(huma.Context) []error {
	transform.ApplyTo(transform.SpecEmail, &b.Email)
	transform.ApplyTo(transform.SpecTrim, &b.OrganizationSlug)
	transform.ApplyTo(transform.SpecTrim, &b.TOTPCode)
	return nil
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

// SelectOrganizationBody names the organization to make active for the session.
type SelectOrganizationBody struct {
	OrganizationSlug string `json:"organization_slug" minLength:"1" maxLength:"64" example:"acme" doc:"Slug of the organization to make active"`
}

// Resolve trims the organization slug before it reaches the service.
func (b *SelectOrganizationBody) Resolve(huma.Context) []error {
	transform.ApplyTo(transform.SpecTrim, &b.OrganizationSlug)
	return nil
}

// SelectOrganizationIn is the huma input type for selecting the active organization.
type SelectOrganizationIn struct {
	Body SelectOrganizationBody
}

// SelectOrganizationOut returns the refreshed bootstrap state and updates the session cookie.
type SelectOrganizationOut struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      BootstrapBody
}

// ForgotPasswordBody carries the email address to send a password reset link to.
type ForgotPasswordBody struct {
	Email string `json:"email" format:"email" minLength:"3" maxLength:"254" example:"user@example.com" doc:"Email address to send the reset link to"`
}

// Resolve normalizes the email before the reset request is processed.
func (b *ForgotPasswordBody) Resolve(huma.Context) []error {
	transform.ApplyTo(transform.SpecEmail, &b.Email)
	return nil
}

// ForgotPasswordIn is the huma input type for requesting a password reset.
type ForgotPasswordIn struct {
	Body ForgotPasswordBody
}

// ForgotPasswordOut always reports acceptance so callers cannot probe for account existence.
type ForgotPasswordOut struct {
	Body struct {
		Accepted bool `json:"accepted"`
	}
}

// CreateInvitationBody carries the invitee email and the role to grant them.
type CreateInvitationBody struct {
	Email string          `json:"email" format:"email" minLength:"3" maxLength:"254" example:"user@example.com" doc:"Email address to invite"`
	Role  permission.Role `json:"role" enum:"member,admin" example:"member" doc:"Role granted to the invited member"`
}

// Resolve normalizes the invitee email; the role is validated by its enum.
func (b *CreateInvitationBody) Resolve(huma.Context) []error {
	transform.ApplyTo(transform.SpecEmail, &b.Email)
	return nil
}

// CreateInvitationIn is the huma input type for creating an invitation.
type CreateInvitationIn struct {
	Body CreateInvitationBody
}

// CreateInvitationOut reports whether the invitation was created.
type CreateInvitationOut struct {
	Body struct {
		Created bool `json:"created"`
	}
}

// AcceptInvitationBody carries the new member's display name and password.
type AcceptInvitationBody struct {
	DisplayName string `json:"display_name" minLength:"1" maxLength:"100" example:"Ada Lovelace" doc:"Display name for the new member"`
	Password    string `json:"password" minLength:"8" maxLength:"128" example:"correct horse battery staple" doc:"Password (minimum 8 characters)"`
}

// Resolve trims the display name before the invitation is accepted.
func (b *AcceptInvitationBody) Resolve(huma.Context) []error {
	transform.ApplyTo(transform.SpecTrim, &b.DisplayName)
	return nil
}

// AcceptInvitationIn is the huma input type for accepting an invitation via its token.
type AcceptInvitationIn struct {
	Token string `path:"token" minLength:"1" doc:"Invitation token from the invite link"`
	Body  AcceptInvitationBody
}

// AcceptInvitationOut returns the bootstrap state and sets the session cookie on success.
type AcceptInvitationOut struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      BootstrapBody
}

// ResetPasswordBody carries the reset token and the new password.
type ResetPasswordBody struct {
	Token    string `json:"token" minLength:"1" doc:"Reset token from the password reset email"`
	Password string `json:"password" minLength:"8" maxLength:"128" example:"correct horse battery staple" doc:"New password (minimum 8 characters)"`
}

// ResetPasswordIn is the huma input type for completing a password reset.
type ResetPasswordIn struct {
	Body ResetPasswordBody
}

// ResetPasswordOut reports whether the password was reset.
type ResetPasswordOut struct {
	Body struct {
		Reset bool `json:"reset"`
	}
}

// TOTPEnrollOut returns the generated secret and otpauth URL for enrolling a TOTP device.
type TOTPEnrollOut struct {
	Body struct {
		Secret string `json:"secret"`
		URL    string `json:"url"`
	}
}

// TOTPConfirmBody carries the code that confirms a pending TOTP enrollment.
type TOTPConfirmBody struct {
	Code string `json:"code" minLength:"6" maxLength:"8" example:"123456" doc:"TOTP code from the authenticator app"`
}

// Resolve trims the confirmation code before it is verified.
func (b *TOTPConfirmBody) Resolve(huma.Context) []error {
	transform.ApplyTo(transform.SpecTrim, &b.Code)
	return nil
}

// TOTPConfirmIn is the huma input type for confirming TOTP enrollment.
type TOTPConfirmIn struct {
	Body TOTPConfirmBody
}

// TOTPConfirmOut reports whether two-factor authentication is now enabled.
type TOTPConfirmOut struct {
	Body struct {
		Enabled bool `json:"enabled"`
	}
}

// LogoutIn is the huma input type for logout; it carries no fields.
type LogoutIn struct{}

// LogoutOut clears the session cookie.
type LogoutOut struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
}

// SessionOut returns the current bootstrap state (same shape as bootstrap).
type SessionOut struct {
	Body BootstrapBody
}

// SetupBody contains the fields required to create the first admin user.
type SetupBody struct {
	Email       string `json:"email" format:"email" minLength:"3" maxLength:"254" example:"admin@example.com" doc:"Admin email address"`
	DisplayName string `json:"display_name" minLength:"1" maxLength:"100" example:"Ada Lovelace" doc:"Display name for the admin user"`
	Password    string `json:"password" minLength:"8" maxLength:"128" example:"correct horse battery staple" doc:"Password (minimum 8 characters)"`
}

// Resolve normalizes the admin email and display name during first-user setup.
func (b *SetupBody) Resolve(huma.Context) []error {
	transform.ApplyTo(transform.SpecEmail, &b.Email)
	transform.ApplyTo(transform.SpecTrim, &b.DisplayName)
	return nil
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
