package workers

import (
	"context"
	"time"

	"github.com/sidarth-23/dinchy/internal/i18n"
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

// TaskName returns the durable task identifier for the session cleanup worker.
func (w *SessionCleanupWorker) TaskName() string {
	return sessionCleanupTaskName
}

// IntervalSeconds returns how often the worker is scheduled to run.
func (w *SessionCleanupWorker) IntervalSeconds() int64 {
	return sessionCleanupIntervalSeconds
}

// LeaseDuration returns how long the worker holds its task lease during a run.
func (w *SessionCleanupWorker) LeaseDuration() time.Duration {
	return sessionCleanupLeaseDuration
}

// RetryDelay returns the delay before the next run after this one completes.
func (w *SessionCleanupWorker) RetryDelay() time.Duration {
	return sessionCleanupRetryDelayDuration
}

// FailureErrorCode returns the error code recorded when a run fails.
func (w *SessionCleanupWorker) FailureErrorCode() string {
	return "task.session_cleanup_failed"
}

// ExecutionCode returns the error code used to classify execution failures.
func (w *SessionCleanupWorker) ExecutionCode() i18n.Code {
	return i18n.CodeWorkersSessionCleanup
}

// Execute deletes ended sessions older than the retention window and reports the count.
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
