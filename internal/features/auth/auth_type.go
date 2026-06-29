package auth

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

//go:generate mockgen -self_package=github.com/sidarth-23/dinchy/internal/features/auth -destination=store_mockdata_test.go -package=auth . Store

type Role string

const RoleAdmin Role = "admin"

type User struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	Role         Role
}

type CreateUserInput struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	Now          time.Time
}

type Session struct {
	ID string
}

type SessionWithUser struct {
	SessionID     string
	UserID        string
	Email         string
	DisplayName   string
	Role          Role
	IdleExpiresAt time.Time
	ExpiresAt     time.Time
	RevokedAt     sql.NullTime
}

type CreateSessionInput struct {
	ID            string
	UserID        string
	TokenHash     string
	IP            string
	UserAgent     string
	Now           time.Time
	IdleExpiresAt time.Time
	ExpiresAt     time.Time
}

type UpdateUserPasswordHashInput struct {
	UserID       string
	PasswordHash string
	Now          time.Time
}

type Store interface {
	CreateFirstUser(ctx context.Context, in CreateUserInput) (User, error)
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUserPasswordHash(ctx context.Context, in UpdateUserPasswordHashInput) error
	CreateSession(ctx context.Context, in CreateSessionInput) (Session, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*SessionWithUser, error)
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error
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
