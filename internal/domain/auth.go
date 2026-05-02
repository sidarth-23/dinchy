package domain

import (
	"database/sql"
	"time"
)

// Session represents a created session record.
type Session struct {
	ID string
}

// SessionWithUser is a session joined with its owner's user info, used for request validation.
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

// CreateSessionInput holds the parameters for creating a new session.
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
