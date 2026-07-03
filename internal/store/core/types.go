package core

import (
	"context"
	"database/sql"
	"time"
)

// DBTX is the common interface satisfied by both *sql.DB and *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type UserRow struct {
	ID          string
	Email       string
	DisplayName string
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
	RevokedAt            sql.NullTime
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

type VerificationTokenRow struct {
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

type TwoFactorRow struct {
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

type UpdateUserPasswordHashParams struct {
	UserID       string
	PasswordHash string
	UpdatedAt    time.Time
}

type InsertSessionParams struct {
	ID                   string
	UserID               string
	ActiveOrganisationID string
	TokenHash            string
	IpAddress            string
	UserAgent            string
	LastSeenAt           time.Time
	IdleExpiresAt        time.Time
	ExpiresAt            time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
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

type TaskParams struct {
	ID                      string
	TaskName                string
	ScheduleIntervalSeconds int64
	NextRunAt               time.Time
	UpdatedAt               time.Time
}

type ClaimTaskParams struct {
	LeaseOwner     string
	LeaseExpiresAt time.Time
	LastRunAt      time.Time
	UpdatedAt      time.Time
	TaskName       string
	NextRunAt      time.Time
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

type TaskStatus string

const (
	TaskStatusOK     TaskStatus = "ok"
	TaskStatusFailed TaskStatus = "failed"
)
