package workers

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
)

// Worker is a registered background task that the runtime can schedule and execute.
type Worker interface {
	TaskName() string
	IntervalSeconds() int64
	LeaseDuration() time.Duration
	RetryDelay() time.Duration
	FailureErrorCode() string
	ExecutionStage() apperrors.Stage
	Execute(ctx context.Context) (WorkerOutcome, error)
}

// WorkerOutcome captures generic execution metadata that the runtime can persist.
type WorkerOutcome struct {
	DeletedCount int64
}
