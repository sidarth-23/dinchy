package tasks

import (
	"context"
	"time"
)

// Store is the data access contract required by the task runtime.
type Store interface {
	EnsureTask(ctx context.Context, name string, intervalSeconds int64, now time.Time) error
	ClaimTask(ctx context.Context, taskName, owner string, leaseUntil, now time.Time) (bool, error)
	FinishTask(ctx context.Context, taskName string, now time.Time, ok bool, errCode, errMsg string, nextRun time.Time) error
	DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error)
}
