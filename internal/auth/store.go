package auth

//go:generate mockgen -destination=mock_store_test.go -package=auth . Store

import (
	"context"

	"github.com/sidarth-23/dinchy/internal/domain"
)

// Store is the data access contract required by the auth service.
type Store interface {
	CreateFirstUser(ctx context.Context, in domain.CreateUserInput) (domain.User, error)
	FindUserByEmail(ctx context.Context, email string) (*domain.User, error)
	CreateSession(ctx context.Context, in domain.CreateSessionInput) (domain.Session, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*domain.SessionWithUser, error)
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error
}
