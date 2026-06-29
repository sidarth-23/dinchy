package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/postgres/sqlcgen"
)

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
	id, err := parseUUID(arg.ID)
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
