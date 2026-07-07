package events

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/workers"
)

type Worker struct {
	service        *Service
	subscriberName string
}

func NewWorker(service *Service, subscriberName string) *Worker {
	return &Worker{service: service, subscriberName: subscriberName}
}

func (w *Worker) TaskName() string {
	return "eventbus_" + w.subscriberName
}

func (w *Worker) IntervalSeconds() int64 {
	if w == nil || w.service == nil {
		return 0
	}
	return int64(w.service.cfg.WorkerInterval / time.Second)
}

func (w *Worker) LeaseDuration() time.Duration {
	return 30 * time.Second
}

func (w *Worker) RetryDelay() time.Duration {
	if w == nil || w.service == nil {
		return 0
	}
	return w.service.cfg.WorkerInterval
}

func (w *Worker) FailureErrorCode() string {
	return "eventbus." + w.subscriberName + ".failed"
}

func (w *Worker) ExecutionStage() apperrors.Stage {
	return apperrors.StageBody
}

func (w *Worker) Execute(ctx context.Context) (workers.WorkerOutcome, error) {
	processed, err := w.service.ProcessSubscriber(ctx, w.subscriberName)
	return workers.WorkerOutcome{DeletedCount: processed}, err
}

var _ workers.Worker = (*Worker)(nil)
