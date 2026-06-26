package sqlite

import (
	"context"
	"database/sql"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

func (q *queries) CountUsers(ctx context.Context) (int64, error) {
	return q.q.CountUsers(ctx)
}

func (q *queries) InsertUser(ctx context.Context, arg core.InsertUserParams) error {
	return q.q.InsertUser(ctx, sqlcgen.InsertUserParams{
		ID:           arg.ID,
		Email:        arg.Email,
		PasswordHash: arg.PasswordHash,
		DisplayName:  arg.DisplayName,
		Role:         arg.Role,
		CreatedAt:    formatTime(arg.CreatedAt),
		UpdatedAt:    formatTime(arg.UpdatedAt),
	})
}

func (q *queries) FindUserByEmail(ctx context.Context, email string) (core.UserRow, error) {
	row, err := q.q.FindUserByEmail(ctx, email)
	if err != nil {
		return core.UserRow{}, err
	}
	return core.UserRow{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		DisplayName:  row.DisplayName,
		Role:         row.Role,
	}, nil
}

func (q *queries) InsertSession(ctx context.Context, arg core.InsertSessionParams) error {
	return q.q.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:            arg.ID,
		UserID:        arg.UserID,
		TokenHash:     arg.TokenHash,
		IpAddress:     arg.IpAddress,
		UserAgent:     arg.UserAgent,
		LastSeenAt:    formatTime(arg.LastSeenAt),
		IdleExpiresAt: formatTime(arg.IdleExpiresAt),
		ExpiresAt:     formatTime(arg.ExpiresAt),
		CreatedAt:     formatTime(arg.CreatedAt),
		UpdatedAt:     formatTime(arg.UpdatedAt),
	})
}

func (q *queries) GetSessionByTokenHash(ctx context.Context, tokenHash string) (core.SessionRow, error) {
	row, err := q.q.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return core.SessionRow{}, err
	}
	idle, err := parseTime(row.IdleExpiresAt)
	if err != nil {
		return core.SessionRow{}, wrapParseErr("idle_expires_at", err)
	}
	exp, err := parseTime(row.ExpiresAt)
	if err != nil {
		return core.SessionRow{}, wrapParseErr("expires_at", err)
	}
	revoked := sql.NullTime{}
	if row.RevokedAt.Valid {
		t, err := parseTime(row.RevokedAt.String)
		if err != nil {
			return core.SessionRow{}, wrapParseErr("revoked_at", err)
		}
		revoked = sql.NullTime{Time: t, Valid: true}
	}
	return core.SessionRow{
		ID:            row.ID,
		UserID:        row.UserID,
		Email:         row.Email,
		DisplayName:   row.DisplayName,
		Role:          row.Role,
		IdleExpiresAt: idle,
		ExpiresAt:     exp,
		RevokedAt:     revoked,
	}, nil
}

func (q *queries) RevokeSessionByTokenHash(ctx context.Context, arg core.RevokeSessionParams) error {
	return q.q.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{
		RevokedAt: sql.NullString{String: formatTime(arg.RevokedAt), Valid: true},
		UpdatedAt: formatTime(arg.UpdatedAt),
		TokenHash: arg.TokenHash,
	})
}
