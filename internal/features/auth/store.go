package auth

//go:generate mockgen -destination=mock_store_test.go -package=auth . Store

import (
	"context"

	"github.com/sidarth-23/dinchy/internal/features/session"
)

// Store is the data access contract required by the auth service.
type Store interface {
	CreateFirstUser(ctx context.Context, in CreateUserInput) (User, error)
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	CreateSession(ctx context.Context, in session.CreateSessionInput) (session.Session, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*session.SessionWithUser, error)
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error
}
