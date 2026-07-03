package auth

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

//go:generate mockgen -self_package=github.com/sidarth-23/dinchy/internal/features/auth -destination=store_mockdata_test.go -package=auth . Store

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type AccountProvider string

const (
	AccountProviderPassword AccountProvider = "password"
)

type VerificationPurpose string

const (
	VerificationPurposePasswordReset VerificationPurpose = "password_reset"
)

type User struct {
	ID          string
	Email       string
	DisplayName string
	Disabled    bool
}

type Account struct {
	ID                string
	UserID            string
	Provider          string
	ProviderAccountID string
	PasswordHash      string
}

type Organisation struct {
	ID   string
	Name string
	Slug string
	Role Role
}

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

type VerificationToken struct {
	ID              string
	UserID          string
	UserIDValid     bool
	Email           string
	Purpose         string
	TokenHash       string
	ExpiresAt       time.Time
	ConsumedAt      time.Time
	ConsumedAtValid bool
}

type CreateUserInput struct {
	ID                   string
	AccountID            string
	OrganisationID       string
	OrganisationMemberID string
	Email                string
	PasswordHash         string
	DisplayName          string
	OrganisationName     string
	OrganisationSlug     string
	Now                  time.Time
}

type Session struct {
	ID string
}

type SessionWithUser struct {
	SessionID        string
	UserID           string
	Email            string
	DisplayName      string
	OrganisationID   string
	OrganisationName string
	OrganisationSlug string
	Role             Role
	IdleExpiresAt    time.Time
	ExpiresAt        time.Time
	RevokedAt        sql.NullTime
}

type CreateSessionInput struct {
	ID             string
	UserID         string
	OrganisationID string
	TokenHash      string
	IP             string
	UserAgent      string
	Now            time.Time
	IdleExpiresAt  time.Time
	ExpiresAt      time.Time
}

type UpdateUserPasswordHashInput struct {
	UserID       string
	PasswordHash string
	Now          time.Time
}

type Store interface {
	CreateFirstUser(ctx context.Context, in CreateUserInput) (User, error)
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	FindPasswordAccountByUserID(ctx context.Context, userID string) (*Account, error)
	FindUserByProviderAccount(ctx context.Context, provider, providerAccountID string) (*User, error)
	ListOrganisationsForUser(ctx context.Context, userID string) ([]Organisation, error)
	FindOrganisationBySlugForUser(ctx context.Context, userID, slug string) (*Organisation, error)
	FindOrganisationByIDForUser(ctx context.Context, userID, organisationID string) (*Organisation, error)
	UpdateUserPasswordHash(ctx context.Context, in UpdateUserPasswordHashInput) error
	CreateSession(ctx context.Context, in CreateSessionInput) (Session, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*SessionWithUser, error)
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error
	RevokeSessionsForUser(ctx context.Context, userID string, now time.Time) error
	CreateVerificationToken(ctx context.Context, token VerificationToken) error
	FindVerificationToken(ctx context.Context, tokenHash, purpose string) (*VerificationToken, error)
	ConsumeVerificationToken(ctx context.Context, tokenID string, now time.Time) error
	SaveTwoFactor(ctx context.Context, in TwoFactor) error
	FindTwoFactorByUserID(ctx context.Context, userID string) (*TwoFactor, error)
	ConfirmTwoFactor(ctx context.Context, userID string, step int64, now time.Time) error
	MarkTwoFactorUsed(ctx context.Context, userID string, step int64, now time.Time) error
	DisableTwoFactor(ctx context.Context, userID string) error
	ListSSOProviderSettings(ctx context.Context) ([]SSOProviderSetting, error)
	UpsertSSOProviderSetting(ctx context.Context, in UpsertSSOProviderSettingInput) error
}

type SSOProviderSetting struct {
	ProviderID    string
	ClientID      string
	ClientIDValid bool
	Secret        string
	SecretValid   bool
	CallbackURL   string
	CallbackValid bool
	Enabled       bool
}

type UpsertSSOProviderSettingInput struct {
	ProviderID    string
	ClientID      string
	ClientIDValid bool
	Secret        string
	SecretValid   bool
	CallbackURL   string
	CallbackValid bool
	Enabled       bool
	Now           time.Time
}

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

type OrganisationOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Role string `json:"role"`
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
	ActiveOrganisation *OrganisationOut  `json:"active_organisation,omitempty"`
	Organisations      []OrganisationOut `json:"organisations,omitempty"`
}

// BootstrapOut is the response type for the bootstrap endpoint.
type BootstrapOut struct {
	Body BootstrapBody
}

// LoginBody contains the credentials required to authenticate.
type LoginBody struct {
	Email            string `json:"email" format:"email" minLength:"3" maxLength:"254" doc:"User email address"`
	Password         string `json:"password" minLength:"1" maxLength:"128" doc:"User password"`
	OrganisationSlug string `json:"organisation_slug,omitempty" doc:"Organisation slug when the user has multiple memberships"`
	TOTPCode         string `json:"totp_code,omitempty" doc:"TOTP code when two-factor authentication is enabled"`
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

type SelectOrganisationBody struct {
	OrganisationSlug string `json:"organisation_slug" minLength:"1"`
}

type SelectOrganisationIn struct {
	Body SelectOrganisationBody
}

type SelectOrganisationOut struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      BootstrapBody
}

type ForgotPasswordBody struct {
	Email string `json:"email" format:"email" minLength:"3" maxLength:"254"`
}

type ForgotPasswordIn struct {
	Body ForgotPasswordBody
}

type ForgotPasswordOut struct {
	Body struct {
		Accepted bool `json:"accepted"`
	}
}

type ResetPasswordBody struct {
	Token    string `json:"token" minLength:"1"`
	Password string `json:"password" minLength:"8" maxLength:"128"`
}

type ResetPasswordIn struct {
	Body ResetPasswordBody
}

type ResetPasswordOut struct {
	Body struct {
		Reset bool `json:"reset"`
	}
}

type TOTPEnrollOut struct {
	Body struct {
		Secret string `json:"secret"`
		URL    string `json:"url"`
	}
}

type TOTPConfirmBody struct {
	Code string `json:"code" minLength:"6" maxLength:"8"`
}

type TOTPConfirmIn struct {
	Body TOTPConfirmBody
}

type TOTPConfirmOut struct {
	Body struct {
		Enabled bool `json:"enabled"`
	}
}

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
