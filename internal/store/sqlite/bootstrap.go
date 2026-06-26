package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

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
