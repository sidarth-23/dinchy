package events

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/workers"
)

// Worker drives one subscriber's event processing on the shared worker runtime.
type Worker struct {
	service        *Service
	subscriberName string
}

// NewWorker returns a Worker that processes events for the named subscriber.
func NewWorker(service *Service, subscriberName string) *Worker {
	return &Worker{service: service, subscriberName: subscriberName}
}

// TaskName returns the worker's unique task identifier.
func (w *Worker) TaskName() string {
	return "eventbus_" + w.subscriberName
}

// IntervalSeconds returns how often the worker runs, in seconds.
func (w *Worker) IntervalSeconds() int64 {
	if w == nil || w.service == nil {
		return 0
	}
	return int64(w.service.cfg.WorkerInterval / time.Second)
}

// LeaseDuration returns how long the worker holds its run lease.
func (w *Worker) LeaseDuration() time.Duration {
	return 30 * time.Second
}

// RetryDelay returns the delay before retrying after a failed run.
func (w *Worker) RetryDelay() time.Duration {
	if w == nil || w.service == nil {
		return 0
	}
	return w.service.cfg.WorkerInterval
}

// FailureErrorCode returns the stable error code reported when a run fails.
func (w *Worker) FailureErrorCode() string {
	return "eventbus." + w.subscriberName + ".failed"
}

// ExecutionStage returns the error stage attributed to the worker's execution.
func (w *Worker) ExecutionStage() apperrors.Stage {
	return apperrors.StageBody
}

// Execute processes one batch of events for the subscriber and reports the outcome.
func (w *Worker) Execute(ctx context.Context) (workers.WorkerOutcome, error) {
	processed, err := w.service.ProcessSubscriber(ctx, w.subscriberName)
	return workers.WorkerOutcome{DeletedCount: processed}, err
}

var _ workers.Worker = (*Worker)(nil)
