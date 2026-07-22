package workers

import (
	"context"
	"time"

	"github.com/sidarth-23/dinchy/internal/i18n"
)

// Worker is a registered background task that the runtime can schedule and execute.
type Worker interface {
	TaskName() string
	IntervalSeconds() int64
	LeaseDuration() time.Duration
	RetryDelay() time.Duration
	FailureErrorCode() string
	ExecutionCode() i18n.Code
	Execute(ctx context.Context) (WorkerOutcome, error)
}

// WorkerOutcome captures generic execution metadata that the runtime can persist.
type WorkerOutcome struct {
	DeletedCount int64
}
