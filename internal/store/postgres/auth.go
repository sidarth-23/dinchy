package postgres

import (
	"context"
	"database/sql"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/postgres/sqlcgen"
)

func (q *queries) CountUsers(ctx context.Context) (int64, error) {
	return q.q.CountUsers(ctx)
}

func (q *queries) InsertUser(ctx context.Context, arg core.InsertUserParams) error {
	id, err := parseUUID(arg.ID)
	if err != nil {
		return err
	}
	return q.q.InsertUser(ctx, sqlcgen.InsertUserParams{
		ID:           id,
		Email:        arg.Email,
		PasswordHash: arg.PasswordHash,
		DisplayName:  arg.DisplayName,
		Role:         arg.Role,
		CreatedAt:    arg.CreatedAt.UTC(),
		UpdatedAt:    arg.UpdatedAt.UTC(),
	})
}

func (q *queries) FindUserByEmail(ctx context.Context, email string) (core.UserRow, error) {
	row, err := q.q.FindUserByEmail(ctx, email)
	if err != nil {
		return core.UserRow{}, err
	}
	return core.UserRow{
		ID:           row.ID.String(),
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		DisplayName:  row.DisplayName,
		Role:         row.Role,
	}, nil
}

func (q *queries) UpdateUserPasswordHash(ctx context.Context, arg core.UpdateUserPasswordHashParams) error {
	id, err := parseUUID(arg.ID)
	if err != nil {
		return err
	}
	return q.q.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
		PasswordHash: arg.PasswordHash,
		UpdatedAt:    arg.UpdatedAt.UTC(),
		ID:           id,
	})
}

func (q *queries) InsertSession(ctx context.Context, arg core.InsertSessionParams) error {
	id, err := parseUUID(arg.ID)
	if err != nil {
		return err
	}
	userID, err := parseUUID(arg.UserID)
	if err != nil {
		return err
	}
	return q.q.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:            id,
		UserID:        userID,
		TokenHash:     arg.TokenHash,
		IpAddress:     arg.IpAddress,
		UserAgent:     arg.UserAgent,
		LastSeenAt:    arg.LastSeenAt.UTC(),
		IdleExpiresAt: arg.IdleExpiresAt.UTC(),
		ExpiresAt:     arg.ExpiresAt.UTC(),
		CreatedAt:     arg.CreatedAt.UTC(),
		UpdatedAt:     arg.UpdatedAt.UTC(),
	})
}

func (q *queries) GetSessionByTokenHash(ctx context.Context, tokenHash string) (core.SessionRow, error) {
	row, err := q.q.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return core.SessionRow{}, err
	}
	return core.SessionRow{
		ID:            row.ID.String(),
		UserID:        row.UserID.String(),
		Email:         row.Email,
		DisplayName:   row.DisplayName,
		Role:          row.Role,
		IdleExpiresAt: row.IdleExpiresAt.UTC(),
		ExpiresAt:     row.ExpiresAt.UTC(),
		RevokedAt:     row.RevokedAt,
	}, nil
}

func (q *queries) RevokeSessionByTokenHash(ctx context.Context, arg core.RevokeSessionParams) error {
	return q.q.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{
		RevokedAt: sql.NullTime{Time: arg.RevokedAt.UTC(), Valid: true},
		UpdatedAt: arg.UpdatedAt.UTC(),
		TokenHash: arg.TokenHash,
	})
}
