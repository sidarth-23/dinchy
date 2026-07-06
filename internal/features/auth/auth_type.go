package auth

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
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

type User struct {
	ID            string
	Email         string
	DisplayName   string
	EmailVerified bool
	Disabled      bool
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

type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusRejected InvitationStatus = "rejected"
	InvitationStatusCanceled InvitationStatus = "canceled"
)

type Invitation struct {
	ID              string
	OrganisationID  string
	Email           string
	Role            Role
	Status          InvitationStatus
	ExpiresAt       time.Time
	InvitedByUserID string
	AcceptedAt      time.Time
	AcceptedAtValid bool
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

type UpdateUserEmailVerifiedAtInput struct {
	UserID string
	Now    time.Time
}

type UserRow struct {
	ID          string
	Email       string
	DisplayName string
}

type AccountRow struct {
	ID                string
	UserID            string
	Provider          string
	ProviderAccountID string
	PasswordHash      string
}

type OrganisationRow struct {
	ID   string
	Name string
	Slug string
	Role string
}

type SSOProviderSettingRow struct {
	ProviderID    string
	ClientID      string
	ClientIDValid bool
	Secret        string
	SecretValid   bool
	CallbackURL   string
	CallbackValid bool
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type TaskParams struct {
	ID                      string
	TaskName                string
	ScheduleIntervalSeconds int64
	NextRunAt               time.Time
	UpdatedAt               time.Time
}

type ClaimTaskParams struct {
	LeaseOwner      string
	LeaseExpiresAt  time.Time
	LastRunAt       time.Time
	UpdatedAt       time.Time
	TaskName        string
	LeaseExpiresAt2 time.Time
	NextRunAt       time.Time
}

type FinishTaskParams struct {
	LastFinishedAt   time.Time
	NextRunAt        time.Time
	LastStatus       string
	LastErrorCode    string
	LastErrorMessage string
	UpdatedAt        time.Time
	TaskName         string
}

type UpdateUserPasswordHashParams struct {
	UserID       string
	PasswordHash string
	UpdatedAt    time.Time
}

type InsertUserParams struct {
	ID              string
	Email           string
	DisplayName     string
	EmailVerifiedAt time.Time
	EmailVerified   bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type InsertAccountParams struct {
	ID                string
	UserID            string
	Provider          string
	ProviderAccountID string
	PasswordHash      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type InsertOrganisationParams struct {
	ID        string
	Name      string
	Slug      string
	Logo      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type InsertOrganisationMemberParams struct {
	ID             string
	OrganisationID string
	UserID         string
	Role           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type InsertVerificationTokenParams struct {
	ID          string
	UserID      string
	UserIDValid bool
	Email       string
	Purpose     string
	TokenHash   string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ConsumeVerificationTokenParams struct {
	ID         string
	ConsumedAt time.Time
	UpdatedAt  time.Time
}

type SaveTwoFactorParams struct {
	ID        string
	UserID    string
	Secret    string
	Verified  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UseTwoFactorParams struct {
	UserID       string
	LastUsedStep int64
	UpdatedAt    time.Time
}

type UpsertSSOProviderSettingParams struct {
	ProviderID    string
	ClientID      string
	ClientIDValid bool
	Secret        string
	SecretValid   bool
	CallbackURL   string
	CallbackValid bool
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type InsertSessionParams struct {
	ID                   string
	UserID               string
	ActiveOrganisationID string
	TokenHash            string
	IPAddress            string
	UserAgent            string
	LastSeenAt           time.Time
	IdleExpiresAt        time.Time
	ExpiresAt            time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type SessionRow struct {
	ID                   string
	UserID               string
	Email                string
	DisplayName          string
	ActiveOrganisationID string
	OrganisationName     string
	OrganisationSlug     string
	Role                 string
	IdleExpiresAt        time.Time
	ExpiresAt            time.Time
	RevokedAt            time.Time
	RevokedAtValid       bool
}

type RevokeSessionParams struct {
	RevokedAt time.Time
	UpdatedAt time.Time
	TokenHash string
}

type RevokeSessionsForUserParams struct {
	RevokedAt time.Time
	UpdatedAt time.Time
	UserID    string
}

type Store interface {
	CountUsers(ctx context.Context) (int64, error)
	InsertUser(ctx context.Context, arg sqlcgen.InsertUserParams) error
	InsertAccount(ctx context.Context, arg sqlcgen.InsertAccountParams) error
	InsertOrganisation(ctx context.Context, arg sqlcgen.InsertOrganisationParams) error
	InsertOrganisationMember(ctx context.Context, arg sqlcgen.InsertOrganisationMemberParams) error
	FindUserByEmail(ctx context.Context, email string) (sqlcgen.FindUserByEmailRow, error)
	UpdateUserEmailVerifiedAt(ctx context.Context, arg sqlcgen.UpdateUserEmailVerifiedAtParams) error
	FindPasswordAccountByUserID(ctx context.Context, userID uuid.UUID) (sqlcgen.FindPasswordAccountByUserIDRow, error)
	FindUserByProviderAccount(ctx context.Context, arg sqlcgen.FindUserByProviderAccountParams) (sqlcgen.FindUserByProviderAccountRow, error)
	ListOrganisationsForUser(ctx context.Context, userID uuid.UUID) ([]sqlcgen.ListOrganisationsForUserRow, error)
	FindOrganisationBySlugForUser(ctx context.Context, arg sqlcgen.FindOrganisationBySlugForUserParams) (sqlcgen.FindOrganisationBySlugForUserRow, error)
	FindOrganisationByIDForUser(ctx context.Context, arg sqlcgen.FindOrganisationByIDForUserParams) (sqlcgen.FindOrganisationByIDForUserRow, error)
	UpdateUserPasswordHash(ctx context.Context, arg sqlcgen.UpdateUserPasswordHashParams) error
	InsertVerificationToken(ctx context.Context, arg sqlcgen.InsertVerificationTokenParams) error
	FindVerificationToken(ctx context.Context, arg sqlcgen.FindVerificationTokenParams) (sqlcgen.FindVerificationTokenRow, error)
	ConsumeVerificationToken(ctx context.Context, arg sqlcgen.ConsumeVerificationTokenParams) error
	InsertOrganisationInvitation(ctx context.Context, arg sqlcgen.InsertOrganisationInvitationParams) error
	FindOrganisationInvitationByToken(ctx context.Context, tokenHash string) (sqlcgen.FindOrganisationInvitationByTokenRow, error)
	FindPendingOrganisationInvitationByEmail(ctx context.Context, arg sqlcgen.FindPendingOrganisationInvitationByEmailParams) (sqlcgen.FindPendingOrganisationInvitationByEmailRow, error)
	ConsumeOrganisationInvitation(ctx context.Context, arg sqlcgen.ConsumeOrganisationInvitationParams) error
	InsertOrReplaceTwoFactor(ctx context.Context, arg sqlcgen.InsertOrReplaceTwoFactorParams) error
	FindTwoFactorByUserID(ctx context.Context, userID uuid.UUID) (sqlcgen.FindTwoFactorByUserIDRow, error)
	ConfirmTwoFactor(ctx context.Context, arg sqlcgen.ConfirmTwoFactorParams) error
	MarkTwoFactorUsed(ctx context.Context, arg sqlcgen.MarkTwoFactorUsedParams) error
	RegisterTwoFactorFailure(ctx context.Context, arg sqlcgen.RegisterTwoFactorFailureParams) error
	DisableTwoFactor(ctx context.Context, userID uuid.UUID) error
	ListSSOProviderSettings(ctx context.Context) ([]sqlcgen.SsoProviderSetting, error)
	UpsertSSOProviderSetting(ctx context.Context, arg sqlcgen.UpsertSSOProviderSettingParams) error
	InsertSession(ctx context.Context, arg sqlcgen.InsertSessionParams) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (sqlcgen.GetSessionByTokenHashRow, error)
	RevokeSessionByTokenHash(ctx context.Context, arg sqlcgen.RevokeSessionByTokenHashParams) error
	RevokeSessionsForUser(ctx context.Context, arg sqlcgen.RevokeSessionsForUserParams) error
	GetInstanceName(ctx context.Context) (string, error)
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

type CreateInvitationBody struct {
	Email string `json:"email" format:"email" minLength:"3" maxLength:"254"`
	Role  string `json:"role" minLength:"1" maxLength:"32"`
}

type CreateInvitationIn struct {
	Body CreateInvitationBody
}

type CreateInvitationOut struct {
	Body struct {
		Created bool `json:"created"`
	}
}

type AcceptInvitationBody struct {
	DisplayName string `json:"display_name" minLength:"1" maxLength:"100"`
	Password    string `json:"password" minLength:"8" maxLength:"128"`
}

type AcceptInvitationIn struct {
	Token string `path:"token" minLength:"1"`
	Body  AcceptInvitationBody
}

type AcceptInvitationOut struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      BootstrapBody
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
