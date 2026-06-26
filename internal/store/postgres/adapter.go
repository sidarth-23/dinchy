package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/postgres/sqlcgen"
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
	id, err := uuid.Parse(arg.ID)
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

func (q *queries) InsertSession(ctx context.Context, arg core.InsertSessionParams) error {
	id, err := uuid.Parse(arg.ID)
	if err != nil {
		return err
	}
	userID, err := uuid.Parse(arg.UserID)
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

func (q *queries) DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := q.q.DeleteEndedSessionsOlderThan(ctx, sqlcgen.DeleteEndedSessionsOlderThanParams{
		ExpiresAt: olderThan.UTC(),
		UpdatedAt: olderThan.UTC(),
	})
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (q *queries) EnsureDefaultSettings(ctx context.Context, now time.Time) error {
	return q.q.EnsureDefaultSettings(ctx, sqlcgen.EnsureDefaultSettingsParams{
		CreatedAt: now.UTC(),
		UpdatedAt: now.UTC(),
	})
}

func (q *queries) GetInstanceName(ctx context.Context) (string, error) {
	return q.q.GetInstanceName(ctx)
}

func (q *queries) EnsureTask(ctx context.Context, arg core.TaskParams) error {
	id, err := uuid.Parse(arg.ID)
	if err != nil {
		return err
	}
	return q.q.EnsureTask(ctx, sqlcgen.EnsureTaskParams{
		ID:                      id,
		TaskName:                arg.TaskName,
		ScheduleIntervalSeconds: arg.ScheduleIntervalSeconds,
		NextRunAt:               sql.NullTime{Time: arg.NextRunAt.UTC(), Valid: true},
		UpdatedAt:               arg.UpdatedAt.UTC(),
	})
}

func (q *queries) ClaimTask(ctx context.Context, arg core.ClaimTaskParams) (int64, error) {
	res, err := q.q.ClaimTask(ctx, sqlcgen.ClaimTaskParams{
		LeaseOwner:       sql.NullString{String: arg.LeaseOwner, Valid: true},
		LeaseExpiresAt:   arg.LeaseExpiresAt.UTC(),
		LastRunAt:        arg.LastRunAt.UTC(),
		UpdatedAt:        arg.UpdatedAt.UTC(),
		TaskName:         arg.TaskName,
		LeaseExpiresAt_2: arg.LastRunAt.UTC(),
		NextRunAt:        arg.NextRunAt.UTC(),
	})
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (q *queries) FinishTask(ctx context.Context, arg core.FinishTaskParams) error {
	return q.q.FinishTask(ctx, sqlcgen.FinishTaskParams{
		LastFinishedAt:   sql.NullTime{Time: arg.LastFinishedAt.UTC(), Valid: true},
		NextRunAt:        sql.NullTime{Time: arg.NextRunAt.UTC(), Valid: true},
		LastStatus:       sql.NullString{String: arg.LastStatus, Valid: true},
		LastErrorCode:    nullString(arg.LastErrorCode),
		LastErrorMessage: nullString(arg.LastErrorMessage),
		UpdatedAt:        arg.UpdatedAt.UTC(),
		TaskName:         arg.TaskName,
	})
}

func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}
