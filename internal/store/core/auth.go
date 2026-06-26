package core

import (
	"context"
	"database/sql"
	"errors"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// CreateFirstUser inserts the initial admin user inside a transaction.
func (s *Store) CreateFirstUser(ctx context.Context, in auth.CreateUserInput) (auth.User, error) {
	var u auth.User
	err := s.WithTx(ctx, func(tx *Store) error {
		count, err := tx.Query().CountUsers(ctx)
		if err != nil {
			return apperrors.Annotate(err,
				apperrors.WithOperation(apperrors.OperationCountUsers),
				apperrors.WithStage(apperrors.StageSetupFirstUser),
			)
		}
		if count > 0 {
			return apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", int(count))))
		}
		if err := tx.Query().InsertUser(ctx, InsertUserParams{
			ID:           in.ID,
			Email:        in.Email,
			PasswordHash: in.PasswordHash,
			DisplayName:  in.DisplayName,
			Role:         string(auth.RoleAdmin),
			CreatedAt:    in.Now.UTC(),
			UpdatedAt:    in.Now.UTC(),
		}); err != nil {
			return apperrors.Annotate(err,
				apperrors.WithOperation(apperrors.OperationInsertUser),
				apperrors.WithStage(apperrors.StageSetupFirstUser),
			)
		}
		u = auth.User{
			ID:           in.ID,
			Email:        in.Email,
			DisplayName:  in.DisplayName,
			Role:         auth.RoleAdmin,
			PasswordHash: in.PasswordHash,
		}
		return nil
	})
	return u, err
}

// FindUserByEmail looks up an active user by email.
func (s *Store) FindUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	row, err := s.Query().FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFindUserByEmail))
	}
	return &auth.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		DisplayName:  row.DisplayName,
		Role:         auth.Role(row.Role),
	}, nil
}

// CreateSession inserts a new session record and returns its ID.
func (s *Store) CreateSession(ctx context.Context, in session.CreateSessionInput) (session.Session, error) {
	if err := s.Query().InsertSession(ctx, InsertSessionParams{
		ID:            in.ID,
		UserID:        in.UserID,
		TokenHash:     in.TokenHash,
		IpAddress:     in.IP,
		UserAgent:     in.UserAgent,
		LastSeenAt:    in.Now.UTC(),
		IdleExpiresAt: in.IdleExpiresAt.UTC(),
		ExpiresAt:     in.ExpiresAt.UTC(),
		CreatedAt:     in.Now.UTC(),
		UpdatedAt:     in.Now.UTC(),
	}); err != nil {
		return session.Session{}, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationCreateSession))
	}
	return session.Session{ID: in.ID}, nil
}

// GetSessionByTokenHash retrieves an active session with its owner's user info.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*session.SessionWithUser, error) {
	row, err := s.Query().GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationGetSessionByTokenHash))
	}
	return &session.SessionWithUser{
		SessionID:     row.ID,
		UserID:        row.UserID,
		Email:         row.Email,
		DisplayName:   row.DisplayName,
		Role:          session.Role(row.Role),
		IdleExpiresAt: row.IdleExpiresAt.UTC(),
		ExpiresAt:     row.ExpiresAt.UTC(),
		RevokedAt:     row.RevokedAt,
	}, nil
}

// RevokeSessionByTokenHash marks the matching session as revoked.
func (s *Store) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	now := time.Now().UTC()
	if err := s.Query().RevokeSessionByTokenHash(ctx, RevokeSessionParams{
		RevokedAt: now,
		UpdatedAt: now,
		TokenHash: tokenHash,
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationRevokeSessionByTokenHash), apperrors.WithTokenHash(apperrors.TokenHash(tokenHash)))
	}
	return nil
}
