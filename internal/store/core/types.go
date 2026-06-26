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
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	Role         string
}

type SessionRow struct {
	ID            string
	UserID        string
	Email         string
	DisplayName   string
	Role          string
	IdleExpiresAt time.Time
	ExpiresAt     time.Time
	RevokedAt     sql.NullTime
}

type InsertUserParams struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type InsertSessionParams struct {
	ID            string
	UserID        string
	TokenHash     string
	IpAddress     string
	UserAgent     string
	LastSeenAt    time.Time
	IdleExpiresAt time.Time
	ExpiresAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RevokeSessionParams struct {
	RevokedAt time.Time
	UpdatedAt time.Time
	TokenHash string
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
