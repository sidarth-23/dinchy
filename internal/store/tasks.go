package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/workers"
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

func (q *queries) ClaimTask(ctx context.Context, arg workers.ClaimTaskParams) (int64, error) {
	res, err := q.q.ClaimTask(ctx, sqlcgen.ClaimTaskParams{
		LeaseOwner:       sql.NullString{String: arg.LeaseOwner, Valid: true},
		LeaseExpiresAt:   clock.NullTime(arg.LeaseExpiresAt, true),
		LastRunAt:        clock.NullTime(arg.LastRunAt, true),
		UpdatedAt:        arg.UpdatedAt.UTC(),
		TaskName:         arg.TaskName,
		LeaseExpiresAt_2: clock.NullTime(arg.LeaseExpiresAt2, true),
		NextRunAt:        clock.NullTime(arg.NextRunAt, true),
	})
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (q *queries) FinishTask(ctx context.Context, arg workers.FinishTaskParams) error {
	return q.q.FinishTask(ctx, sqlcgen.FinishTaskParams{
		LastFinishedAt:   clock.NullTime(arg.LastFinishedAt, true),
		NextRunAt:        clock.NullTime(arg.NextRunAt, true),
		LastStatus:       sql.NullString{String: arg.LastStatus, Valid: true},
		LastErrorCode:    nullString(arg.LastErrorCode),
		LastErrorMessage: nullString(arg.LastErrorMessage),
		UpdatedAt:        arg.UpdatedAt.UTC(),
		TaskName:         arg.TaskName,
	})
}
