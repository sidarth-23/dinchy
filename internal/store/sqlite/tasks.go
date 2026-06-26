package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

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
