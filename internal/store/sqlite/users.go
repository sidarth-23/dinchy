package sqlite

import (
	"context"
	"database/sql"
	"errors"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

// CreateFirstUser inserts the initial admin user inside a transaction that first
// verifies no users exist, making setup race-safe.
func (s *Store) CreateFirstUser(ctx context.Context, in auth.CreateUserInput) (auth.User, error) {
	var u auth.User
	err := s.WithTx(ctx, func(tx *Store) error {
		count, err := tx.q.CountUsers(ctx)
		if err != nil {
			return err
		}
		if count > 0 {
			return apperrors.SetupCompleted(
				apperrors.WithMeta("resource", "users"),
				apperrors.WithMeta("count", count),
			)
		}
		now := tsFormat(in.Now)
		if err := tx.q.InsertUser(ctx, sqlcgen.InsertUserParams{
			ID:           in.ID,
			Email:        in.Email,
			PasswordHash: in.PasswordHash,
			DisplayName:  in.DisplayName,
			Role:         string(auth.RoleAdmin),
			CreatedAt:    now,
			UpdatedAt:    now,
		}); err != nil {
			return err
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

// FindUserByEmail looks up an active (non-disabled) user by their canonical email address.
func (s *Store) FindUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	row, err := s.q.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Internal(err, apperrors.WithMeta("operation", "FindUserByEmail"))
	}
	return &auth.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		DisplayName:  row.DisplayName,
		Role:         auth.Role(row.Role),
	}, nil
}
