package auth

import (
	"database/sql"
	"time"
)

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
