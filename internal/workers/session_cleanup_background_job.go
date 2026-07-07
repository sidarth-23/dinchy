package workers

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

const (
	sessionCleanupTaskName           = "session_cleanup"
	sessionCleanupIntervalSeconds    = int64(300)
	sessionCleanupLeaseDuration      = 15 * time.Second
	sessionCleanupRetryDelayDuration = 5 * time.Minute
	sessionCleanupRetentionDuration  = 24 * time.Hour
)

// SessionCleanupWorker prunes ended sessions after the retention period.
type SessionCleanupWorker struct {
	store Store
	clock clock.Clock
}

// NewSessionCleanupWorker creates the session cleanup worker.
func NewSessionCleanupWorker(store Store, clk clock.Clock) Worker {
	return &SessionCleanupWorker{store: store, clock: clk}
}

func (w *SessionCleanupWorker) TaskName() string {
	return sessionCleanupTaskName
}

func (w *SessionCleanupWorker) IntervalSeconds() int64 {
	return sessionCleanupIntervalSeconds
}

func (w *SessionCleanupWorker) LeaseDuration() time.Duration {
	return sessionCleanupLeaseDuration
}

func (w *SessionCleanupWorker) RetryDelay() time.Duration {
	return sessionCleanupRetryDelayDuration
}

func (w *SessionCleanupWorker) FailureErrorCode() string {
	return "task.session_cleanup_failed"
}

func (w *SessionCleanupWorker) ExecutionStage() apperrors.Stage {
	return apperrors.StageDeleteEndedSessions
}

func (w *SessionCleanupWorker) Execute(ctx context.Context) (WorkerOutcome, error) {
	now := w.clock.Now()
	result, err := w.store.DeleteEndedSessionsOlderThan(ctx, sqlcgen.DeleteEndedSessionsOlderThanParams{
		ExpiresAt: sqltype.Timestamptz(now.Add(-sessionCleanupRetentionDuration)),
		UpdatedAt: sqltype.Timestamptz(now),
	})
	if err != nil {
		return WorkerOutcome{}, err
	}
	return WorkerOutcome{DeletedCount: result.RowsAffected()}, nil
}
