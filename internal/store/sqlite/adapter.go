package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

type queries struct {
	q *sqlcgen.Queries
}

func newQueries(q *sqlcgen.Queries) core.Queries {
	return &queries{q: q}
}

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

func (q *queries) DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := q.q.DeleteEndedSessionsOlderThan(ctx, sqlcgen.DeleteEndedSessionsOlderThanParams{
		ExpiresAt: formatTime(olderThan),
		UpdatedAt: formatTime(olderThan),
	})
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (q *queries) EnsureDefaultSettings(ctx context.Context, now time.Time) error {
	return q.q.EnsureDefaultSettings(ctx, sqlcgen.EnsureDefaultSettingsParams{
		CreatedAt: formatTime(now),
		UpdatedAt: formatTime(now),
	})
}

func (q *queries) GetInstanceName(ctx context.Context) (string, error) {
	return q.q.GetInstanceName(ctx)
}

func (q *queries) EnsureTask(ctx context.Context, arg core.TaskParams) error {
	return q.q.EnsureTask(ctx, sqlcgen.EnsureTaskParams{
		ID:                      arg.ID,
		TaskName:                arg.TaskName,
		ScheduleIntervalSeconds: arg.ScheduleIntervalSeconds,
		NextRunAt:               sql.NullString{String: formatTime(arg.NextRunAt), Valid: true},
		UpdatedAt:               formatTime(arg.UpdatedAt),
	})
}

func (q *queries) ClaimTask(ctx context.Context, arg core.ClaimTaskParams) (int64, error) {
	res, err := q.q.ClaimTask(ctx, sqlcgen.ClaimTaskParams{
		LeaseOwner:       sql.NullString{String: arg.LeaseOwner, Valid: true},
		LeaseExpiresAt:   sql.NullString{String: formatTime(arg.LeaseExpiresAt), Valid: true},
		LastRunAt:        sql.NullString{String: formatTime(arg.LastRunAt), Valid: true},
		UpdatedAt:        formatTime(arg.UpdatedAt),
		TaskName:         arg.TaskName,
		LeaseExpiresAt_2: sql.NullString{String: formatTime(arg.LastRunAt), Valid: true},
		NextRunAt:        sql.NullString{String: formatTime(arg.NextRunAt), Valid: true},
	})
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (q *queries) FinishTask(ctx context.Context, arg core.FinishTaskParams) error {
	return q.q.FinishTask(ctx, sqlcgen.FinishTaskParams{
		LastFinishedAt:   sql.NullString{String: formatTime(arg.LastFinishedAt), Valid: true},
		NextRunAt:        sql.NullString{String: formatTime(arg.NextRunAt), Valid: true},
		LastStatus:       sql.NullString{String: arg.LastStatus, Valid: true},
		LastErrorCode:    nullString(arg.LastErrorCode),
		LastErrorMessage: nullString(arg.LastErrorMessage),
		UpdatedAt:        formatTime(arg.UpdatedAt),
		TaskName:         arg.TaskName,
	})
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(v string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func wrapParseErr(field string, err error) error {
	return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("invalid sqlite timestamp for %s: %w", field, err)))
}
