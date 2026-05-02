package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/sidarth-23/dinchy/internal/domain"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

// CreateFirstUser inserts the initial admin user inside a transaction that first
// verifies no users exist, making setup race-safe.
func (s *Store) CreateFirstUser(ctx context.Context, in domain.CreateUserInput) (domain.User, error) {
	var u domain.User
	err := s.WithTx(ctx, func(tx *Store) error {
		count, err := tx.q.CountUsers(ctx)
		if err != nil {
			return err
		}
		if count > 0 {
			return errors.New("setup already completed")
		}
		now := tsFormat(in.Now)
		if err := tx.q.InsertUser(ctx, sqlcgen.InsertUserParams{
			ID:           in.ID,
			Email:        in.Email,
			PasswordHash: in.PasswordHash,
			DisplayName:  in.DisplayName,
			Role:         string(domain.RoleAdmin),
			CreatedAt:    now,
			UpdatedAt:    now,
		}); err != nil {
			return err
		}
		u = domain.User{
			ID:           in.ID,
			Email:        in.Email,
			DisplayName:  in.DisplayName,
			Role:         domain.RoleAdmin,
			PasswordHash: in.PasswordHash,
		}
		return nil
	})
	return u, err
}

// FindUserByEmail looks up an active (non-disabled) user by their canonical email address.
func (s *Store) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := s.q.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &domain.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		DisplayName:  row.DisplayName,
		Role:         domain.Role(row.Role),
	}, nil
}
