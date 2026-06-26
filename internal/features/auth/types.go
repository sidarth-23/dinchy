package auth

import (
	"time"

	"github.com/sidarth-23/dinchy/internal/features/session"
)

type Role = session.Role

const RoleAdmin = session.RoleAdmin

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
