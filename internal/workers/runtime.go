// Package workers provides durable background workers backed by the store.
package workers

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

// Runtime manages periodic background workers with lease-based execution.
type Runtime struct {
	store   Store
	clock   clock.Clock
	logger  *slog.Logger
	owner   string
	cancel  context.CancelFunc
	errCh   chan<- error
	workers []Worker
}

// NewRuntime creates a worker runtime with the given store, clock, logger, and registered workers.
func NewRuntime(s Store, clk clock.Clock, logger *slog.Logger, errCh chan<- error, registeredWorkers ...Worker) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{store: s, clock: clk, logger: logger, owner: "local", errCh: errCh, workers: registeredWorkers}
}

// Start registers all workers and begins the background ticker loop.
func (r *Runtime) Start(ctx context.Context) error {
	if len(r.workers) == 0 {
		logging.Warn(ctx, r.logger, "No workers registered")
	}
	for _, worker := range r.workers {
		if err := r.registerWorker(ctx, worker); err != nil {
			return err
		}
	}
	cctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	go r.loop(cctx)
	logging.Info(ctx, r.logger, "Worker runtime started",
		slog.Int("worker_count", len(r.workers)),
	)
	return nil
}

// Stop cancels the background loop.
func (r *Runtime) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	logging.Info(context.Background(), r.logger, "Worker runtime stopping")
}

func (r *Runtime) loop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	r.runAllWorkers(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runAllWorkers(ctx)
		}
	}
}

func (r *Runtime) runAllWorkers(ctx context.Context) {
	for _, worker := range r.workers {
		if err := r.runWorker(ctx, worker); err != nil {
			if r.errCh != nil {
				r.errCh <- err
			}
			return
		}
	}
}

func (r *Runtime) registerWorker(ctx context.Context, worker Worker) error {
	now := r.clock.Now()
	if err := r.store.EnsureTask(ctx, sqlcgen.EnsureTaskParams{
		ID:                      uuid.NewSHA1(uuid.Nil, []byte(worker.TaskName())),
		TaskName:                worker.TaskName(),
		ScheduleIntervalSeconds: worker.IntervalSeconds(),
		NextRunAt:               sqltype.Timestamptz(now),
		UpdatedAt:               sqltype.Timestamptz(now),
	}); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.Task(worker.TaskName())),
			apperrors.WithStage(apperrors.StageEnsureTask),
		)
	}
	logging.Info(ctx, r.logger, "Registered worker",
		slog.String("task", worker.TaskName()),
		slog.Int64("interval_seconds", worker.IntervalSeconds()),
	)
	return nil
}

func (r *Runtime) runWorker(ctx context.Context, worker Worker) error {
	now := r.clock.Now()
	leaseExpiresAt := now.Add(worker.LeaseDuration())
	result, err := r.store.ClaimTask(ctx, sqlcgen.ClaimTaskParams{
		LeaseOwner:           sqltype.Text(r.owner),
		LeaseExpiresAt:       sqltype.Timestamptz(leaseExpiresAt),
		LastRunAt:            sqltype.Timestamptz(now),
		UpdatedAt:            sqltype.Timestamptz(now),
		TaskName:             worker.TaskName(),
		LeaseExpiresAtCutoff: sqltype.Timestamptz(leaseExpiresAt),
		NextRunAtCutoff:      sqltype.Timestamptz(now),
	})
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.Task(worker.TaskName())),
			apperrors.WithStage(apperrors.StageClaimTask),
		)
	}
	if result.RowsAffected() == 0 {
		return nil
	}

	outcome, executeErr := worker.Execute(ctx)
	if executeErr != nil {
		if finishErr := r.store.FinishTask(ctx, sqlcgen.FinishTaskParams{
			LastFinishedAt:   sqltype.Timestamptz(now),
			NextRunAt:        sqltype.Timestamptz(now.Add(worker.RetryDelay())),
			LastStatus:       sqltype.Text("failed"),
			LastErrorCode:    sqltype.Text(worker.FailureErrorCode()),
			LastErrorMessage: sqltype.Text(executeErr.Error()),
			UpdatedAt:        sqltype.Timestamptz(now),
			TaskName:         worker.TaskName(),
		}); finishErr != nil {
			return apperrors.Annotate(finishErr,
				apperrors.WithTask(apperrors.Task(worker.TaskName())),
				apperrors.WithStage(apperrors.StageFinishFailedRun),
			)
		}
		return apperrors.Annotate(executeErr,
			apperrors.WithTask(apperrors.Task(worker.TaskName())),
			apperrors.WithStage(worker.ExecutionStage()),
		)
	}

	if err := r.store.FinishTask(ctx, sqlcgen.FinishTaskParams{
		LastFinishedAt: sqltype.Timestamptz(now),
		NextRunAt:      sqltype.Timestamptz(now.Add(worker.RetryDelay())),
		LastStatus:     sqltype.Text("succeeded"),
		UpdatedAt:      sqltype.Timestamptz(now),
		TaskName:       worker.TaskName(),
	}); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.Task(worker.TaskName())),
			apperrors.WithStage(apperrors.StageFinishSuccess),
			apperrors.WithDeletedCount(apperrors.DeletedCount(outcome.DeletedCount)),
		)
	}
	if outcome.DeletedCount > 0 {
		logging.Info(ctx, r.logger, "Completed worker run",
			slog.String("task", worker.TaskName()),
			slog.Int64("affected_count", outcome.DeletedCount),
		)
	}
	return nil
}
