package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

// EnsureTask registers a task by name if it does not already exist.
func (s *Store) EnsureTask(ctx context.Context, name string, intervalSeconds int64, now time.Time) error {
	return s.q.EnsureTask(ctx, sqlcgen.EnsureTaskParams{
		ID:                      "task_" + name,
		TaskName:                name,
		ScheduleIntervalSeconds: intervalSeconds,
		NextRunAt:               sql.NullString{String: tsFormat(now), Valid: true},
		UpdatedAt:               tsFormat(now),
	})
}

// ClaimTask atomically acquires the lease on a task that is due to run.
// Returns true if the claim succeeded.
func (s *Store) ClaimTask(ctx context.Context, taskName, owner string, leaseUntil, now time.Time) (bool, error) {
	nowStr := tsFormat(now)
	res, err := s.q.ClaimTask(ctx, sqlcgen.ClaimTaskParams{
		LeaseOwner:       sql.NullString{String: owner, Valid: true},
		LeaseExpiresAt:   sql.NullString{String: tsFormat(leaseUntil), Valid: true},
		LeaseExpiresAt_2: sql.NullString{String: nowStr, Valid: true},
		LastRunAt:        sql.NullString{String: nowStr, Valid: true},
		UpdatedAt:        nowStr,
		TaskName:         taskName,
		NextRunAt:        sql.NullString{String: nowStr, Valid: true},
	})
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// FinishTask records the outcome of a completed task run and schedules its next execution.
func (s *Store) FinishTask(ctx context.Context, taskName string, now time.Time, ok bool, errCode, errMsg string, nextRun time.Time) error {
	status := "failed"
	if ok {
		status = "ok"
	}
	nowStr := tsFormat(now)
	return s.q.FinishTask(ctx, sqlcgen.FinishTaskParams{
		LastFinishedAt:   sql.NullString{String: nowStr, Valid: true},
		NextRunAt:        sql.NullString{String: tsFormat(nextRun), Valid: true},
		LastStatus:       sql.NullString{String: status, Valid: true},
		LastErrorCode:    nullString(errCode),
		LastErrorMessage: nullString(errMsg),
		UpdatedAt:        nowStr,
		TaskName:         taskName,
	})
}
