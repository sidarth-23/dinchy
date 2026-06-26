package sqlite

import (
	"context"
	"database/sql"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

// EnsureTask registers a task by name if it does not already exist.
func (s *Store) EnsureTask(ctx context.Context, name string, intervalSeconds int64, now time.Time) error {
	if err := s.q.EnsureTask(ctx, sqlcgen.EnsureTaskParams{
		ID:                      "task_" + name,
		TaskName:                name,
		ScheduleIntervalSeconds: intervalSeconds,
		NextRunAt:               sql.NullString{String: tsFormat(now), Valid: true},
		UpdatedAt:               tsFormat(now),
	}); err != nil {
		return apperrors.Internal(err, apperrors.WithMeta("operation", "EnsureTask"), apperrors.WithMeta("task", name))
	}
	return nil
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
		return false, apperrors.Internal(err, apperrors.WithMeta("operation", "ClaimTask"), apperrors.WithMeta("task", taskName))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, apperrors.Internal(err, apperrors.WithMeta("operation", "ClaimTask"), apperrors.WithMeta("task", taskName), apperrors.WithMeta("stage", "rows_affected"))
	}
	return n > 0, nil
}

// FinishTask records the outcome of a completed task run and schedules its next execution.
func (s *Store) FinishTask(ctx context.Context, taskName string, now time.Time, ok bool, errCode, errMsg string, nextRun time.Time) error {
	status := "failed"
	if ok {
		status = "ok"
	}
	nowStr := tsFormat(now)
	if err := s.q.FinishTask(ctx, sqlcgen.FinishTaskParams{
		LastFinishedAt:   sql.NullString{String: nowStr, Valid: true},
		NextRunAt:        sql.NullString{String: tsFormat(nextRun), Valid: true},
		LastStatus:       sql.NullString{String: status, Valid: true},
		LastErrorCode:    nullString(errCode),
		LastErrorMessage: nullString(errMsg),
		UpdatedAt:        nowStr,
		TaskName:         taskName,
	}); err != nil {
		return apperrors.Internal(err, apperrors.WithMeta("operation", "FinishTask"), apperrors.WithMeta("task", taskName))
	}
	return nil
}
