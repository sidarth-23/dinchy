package store

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/store/types"
)

// DeleteEndedSessionsOlderThan removes ended sessions older than the cutoff.
func (s *Store) DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error) {
	count, err := s.Query().DeleteEndedSessionsOlderThan(ctx, olderThan)
	if err != nil {
		return 0, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationDeleteEndedSessionsOlderThan))
	}
	return count, nil
}

func (s *Store) EnsureTask(ctx context.Context, taskName string, intervalSeconds int64, now time.Time) error {
	if err := s.Query().EnsureTask(ctx, types.TaskParams{
		ID:                      taskIDForName(taskName),
		TaskName:                taskName,
		ScheduleIntervalSeconds: intervalSeconds,
		UpdatedAt:               now.UTC(),
		NextRunAt:               now.UTC(),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationEnsureTask))
	}
	return nil
}

func (s *Store) ClaimTask(ctx context.Context, taskName, leaseOwner string, leaseExpiresAt, now time.Time) (bool, error) {
	count, err := s.Query().ClaimTask(ctx, types.ClaimTaskParams{
		TaskName:       taskName,
		LeaseOwner:     leaseOwner,
		LeaseExpiresAt: leaseExpiresAt.UTC(),
		LastRunAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
		NextRunAt:      now.UTC(),
	})
	if err != nil {
		return false, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationClaimTask))
	}
	return count > 0, nil
}

func (s *Store) FinishTask(ctx context.Context, taskName string, lastFinishedAt time.Time, succeeded bool, errorCode, errorMessage string, nextRunAt time.Time) error {
	lastStatus := "succeeded"
	if !succeeded {
		lastStatus = "failed"
	}
	if err := s.Query().FinishTask(ctx, types.FinishTaskParams{
		TaskName:         taskName,
		LastFinishedAt:   lastFinishedAt.UTC(),
		NextRunAt:        nextRunAt.UTC(),
		LastStatus:       lastStatus,
		LastErrorCode:    errorCode,
		LastErrorMessage: errorMessage,
		UpdatedAt:        nextRunAt.UTC(),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFinishTask))
	}
	return nil
}
