package store

import (
	"context"
	"time"

	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/store/types"
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

func (q *queries) EnsureTask(ctx context.Context, arg types.TaskParams) error {
	parsedID, err := id.Parse(arg.ID)
	if err != nil {
		return err
	}
	return q.q.EnsureTask(ctx, sqlcgen.EnsureTaskParams{
		ID:                      parsedID,
		TaskName:                arg.TaskName,
		ScheduleIntervalSeconds: arg.ScheduleIntervalSeconds,
		NextRunAt:               clock.NullTime(arg.NextRunAt, true),
		UpdatedAt:               arg.UpdatedAt.UTC(),
	})
}
