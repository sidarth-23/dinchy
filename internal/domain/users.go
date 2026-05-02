// Package domain defines the core business types shared across all layers.
// This package has zero project imports — it depends only on the standard library.
package domain

import "time"

// Role represents an authorization role for a user account.
type Role string

// RoleAdmin is the initial role granted to the first registered user.
const RoleAdmin Role = "admin"

// User represents an authenticated user account.
type User struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	Role         Role
}

// CreateUserInput holds the parameters for creating the initial admin user.
type CreateUserInput struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	Now          time.Time
}
