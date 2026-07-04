package audit

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/workers"
)

type Worker struct {
	service         *Service
	intervalSeconds int64
}

func NewWorker(service *Service, intervalSeconds int64) *Worker {
	return &Worker{service: service, intervalSeconds: intervalSeconds}
}

func (w *Worker) TaskName() string                { return "audit_stream_persist" }
func (w *Worker) IntervalSeconds() int64          { return w.intervalSeconds }
func (w *Worker) LeaseDuration() time.Duration    { return 30 * time.Second }
func (w *Worker) RetryDelay() time.Duration       { return time.Duration(w.intervalSeconds) * time.Second }
func (w *Worker) FailureErrorCode() string        { return "audit.persist_failed" }
func (w *Worker) ExecutionStage() apperrors.Stage { return apperrors.StageBody }

func (w *Worker) Execute(ctx context.Context) (workers.WorkerOutcome, error) {
	processed, err := w.service.Process(ctx)
	return workers.WorkerOutcome{DeletedCount: processed}, err
}
