package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/postgres/sqlcgen"
)

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

func (q *queries) ClaimTask(ctx context.Context, arg core.ClaimTaskParams) (int64, error) {
	res, err := q.q.ClaimTask(ctx, sqlcgen.ClaimTaskParams{
		LeaseOwner:       sql.NullString{String: arg.LeaseOwner, Valid: true},
		LeaseExpiresAt:   sql.NullTime{Time: arg.LeaseExpiresAt.UTC(), Valid: true},
		LastRunAt:        sql.NullTime{Time: arg.LastRunAt.UTC(), Valid: true},
		UpdatedAt:        arg.UpdatedAt.UTC(),
		TaskName:         arg.TaskName,
		LeaseExpiresAt_2: sql.NullTime{Time: arg.LastRunAt.UTC(), Valid: true},
		NextRunAt:        sql.NullTime{Time: arg.NextRunAt.UTC(), Valid: true},
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
