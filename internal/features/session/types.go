package session

import (
	"database/sql"
	"time"
)

type Role string

const RoleAdmin Role = "admin"

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
