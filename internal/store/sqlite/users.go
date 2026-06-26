package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

// CreateFirstUser inserts the initial admin user inside a transaction that first
// verifies no users exist, making setup race-safe.
func (s *Store) CreateFirstUser(ctx context.Context, in auth.CreateUserInput) (auth.User, error) {
	var u auth.User
	err := s.WithTx(ctx, func(tx *Store) error {
		count, err := tx.q.CountUsers(ctx)
		if err != nil {
			return apperrors.Annotate(err,
				apperrors.WithMeta("operation", "CountUsers"),
				apperrors.WithMeta("stage", "setup_first_user"),
			)
		}
		if count > 0 {
			return apperrors.New(http.StatusConflict, i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", int(count))))
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
			return apperrors.Annotate(err,
				apperrors.WithMeta("operation", "InsertUser"),
				apperrors.WithMeta("stage", "setup_first_user"),
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

// FindUserByEmail looks up an active (non-disabled) user by their canonical email address.
func (s *Store) FindUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	row, err := s.q.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err,
			apperrors.WithMeta("operation", "FindUserByEmail"),
		)
	}
	return &auth.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		DisplayName:  row.DisplayName,
		Role:         auth.Role(row.Role),
	}, nil
}
