package workers

import (
	"context"
	"errors"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
)

const (
	sessionCleanupTaskName           = "session_cleanup"
	sessionCleanupIntervalSeconds    = int64(300)
	sessionCleanupLeaseDuration      = 15 * time.Second
	sessionCleanupRetryDelayDuration = 5 * time.Minute
	sessionCleanupRetentionDuration  = 24 * time.Hour
)

func (r *Runtime) runSessionCleanup(ctx context.Context) error {
	now := r.clock.Now()
	ok, err := r.store.ClaimTask(ctx, sessionCleanupTaskName, r.owner, now.Add(sessionCleanupLeaseDuration), now)
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.TaskSessionCleanup),
			apperrors.WithStage(apperrors.StageClaimTask),
		)
	}
	if !ok {
		return nil
	}

	deletedCount, cleanupErr := r.store.DeleteEndedSessionsOlderThan(ctx, now.Add(-sessionCleanupRetentionDuration))
	if cleanupErr != nil {
		if finishErr := r.store.FinishTask(ctx, sessionCleanupTaskName, now, false, "task.session_cleanup_failed", cleanupErr.Error(), now.Add(sessionCleanupRetryDelayDuration)); finishErr != nil {
			return errors.Join(
				apperrors.Annotate(cleanupErr,
					apperrors.WithTask(apperrors.TaskSessionCleanup),
					apperrors.WithStage(apperrors.StageDeleteEndedSessions),
				),
				apperrors.Annotate(finishErr,
					apperrors.WithTask(apperrors.TaskSessionCleanup),
					apperrors.WithStage(apperrors.StageFinishFailedRun),
				),
			)
		}
		return apperrors.Annotate(cleanupErr,
			apperrors.WithTask(apperrors.TaskSessionCleanup),
			apperrors.WithStage(apperrors.StageDeleteEndedSessions),
		)
	}

	if err := r.store.FinishTask(ctx, sessionCleanupTaskName, now, true, "", "", now.Add(sessionCleanupRetryDelayDuration)); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.TaskSessionCleanup),
			apperrors.WithStage(apperrors.StageFinishSuccess),
			apperrors.WithDeletedCount(apperrors.DeletedCount(deletedCount)),
		)
	}
	return nil
}
