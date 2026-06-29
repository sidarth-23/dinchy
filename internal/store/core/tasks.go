package core

import (
	"context"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// EnsureTask registers a task by name if it does not already exist.
func (s *Store) EnsureTask(ctx context.Context, name string, intervalSeconds int64, now time.Time) error {
	taskID, err := uuid.NewV7()
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(err), apperrors.WithOperation(apperrors.OperationEnsureTask), apperrors.WithTask(apperrors.Task(name)))
	}
	if err := s.Query().EnsureTask(ctx, TaskParams{
		ID:                      taskID.String(),
		TaskName:                name,
		ScheduleIntervalSeconds: intervalSeconds,
		NextRunAt:               now.UTC(),
		UpdatedAt:               now.UTC(),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationEnsureTask), apperrors.WithTask(apperrors.Task(name)))
	}
	return nil
}

// ClaimTask atomically acquires the lease on a task that is due to run.
func (s *Store) ClaimTask(ctx context.Context, taskName, owner string, leaseUntil, now time.Time) (bool, error) {
	n, err := s.Query().ClaimTask(ctx, ClaimTaskParams{
		LeaseOwner:     owner,
		LeaseExpiresAt: leaseUntil.UTC(),
		LastRunAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
		TaskName:       taskName,
		NextRunAt:      now.UTC(),
	})
	if err != nil {
		return false, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationClaimTask), apperrors.WithTask(apperrors.Task(taskName)))
	}
	return n > 0, nil
}

// FinishTask records the outcome of a completed task run.
func (s *Store) FinishTask(ctx context.Context, taskName string, now time.Time, ok bool, errCode, errMsg string, nextRun time.Time) error {
	status := TaskStatusFailed
	if ok {
		status = TaskStatusOK
	}
	if err := s.Query().FinishTask(ctx, FinishTaskParams{
		LastFinishedAt:   now.UTC(),
		NextRunAt:        nextRun.UTC(),
		LastStatus:       string(status),
		LastErrorCode:    errCode,
		LastErrorMessage: errMsg,
		UpdatedAt:        now.UTC(),
		TaskName:         taskName,
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFinishTask), apperrors.WithTask(apperrors.Task(taskName)))
	}
	return nil
}

// DeleteEndedSessionsOlderThan deletes ended sessions that were updated before the cutoff.
func (s *Store) DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error) {
	n, err := s.Query().DeleteEndedSessionsOlderThan(ctx, olderThan.UTC())
	if err != nil {
		return 0, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationDeleteEndedSessionsOlderThan))
	}
	return n, nil
}
