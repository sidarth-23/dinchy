package auth

//go:generate mockgen -self_package=github.com/sidarth-23/dinchy/internal/features/auth -destination=store_mockdata_test.go -package=auth . Store

import (
	"context"
	"time"

	"github.com/sidarth-23/dinchy/internal/features/session"
)

// Store is the data access contract required by the auth service.
type Store interface {
	CreateFirstUser(ctx context.Context, in CreateUserInput) (User, error)
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUserPasswordHash(ctx context.Context, in UpdateUserPasswordHashInput) error
	CreateSession(ctx context.Context, in session.CreateSessionInput) (session.Session, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*session.SessionWithUser, error)
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error
}

type UpdateUserPasswordHashInput struct {
	UserID       string
	PasswordHash string
	Now          time.Time
}
