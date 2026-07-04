// Package workers provides durable background workers backed by the store.
package workers

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
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
			r.report(err)
			return
		}
	}
}

func (r *Runtime) report(err error) {
	if r.errCh == nil || err == nil {
		return
	}
	r.errCh <- err
}

func (r *Runtime) registerWorker(ctx context.Context, worker Worker) error {
	now := r.clock.Now()
	if err := r.store.EnsureTask(ctx, sqlcgen.EnsureTaskParams{
		ID:                      taskIDForName(worker.TaskName()),
		TaskName:                worker.TaskName(),
		ScheduleIntervalSeconds: worker.IntervalSeconds(),
		NextRunAt:               sql.NullTime{Time: now.UTC(), Valid: true},
		UpdatedAt:               now.UTC(),
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
	result, err := r.store.ClaimTask(ctx, sqlcgen.ClaimTaskParams{
		LeaseOwner:       sql.NullString{String: r.owner, Valid: true},
		LeaseExpiresAt:   sql.NullTime{Time: now.Add(worker.LeaseDuration()).UTC(), Valid: true},
		LastRunAt:        sql.NullTime{Time: now.UTC(), Valid: true},
		UpdatedAt:        now.UTC(),
		TaskName:         worker.TaskName(),
		LeaseExpiresAt_2: sql.NullTime{Time: now.Add(worker.LeaseDuration()).UTC(), Valid: true},
		NextRunAt:        sql.NullTime{Time: now.UTC(), Valid: true},
	})
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.Task(worker.TaskName())),
			apperrors.WithStage(apperrors.StageClaimTask),
		)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.Task(worker.TaskName())),
			apperrors.WithStage(apperrors.StageClaimTask),
		)
	}
	if rowsAffected == 0 {
		return nil
	}

	outcome, executeErr := worker.Execute(ctx)
	if executeErr != nil {
		if finishErr := r.store.FinishTask(ctx, sqlcgen.FinishTaskParams{
			LastFinishedAt:   sql.NullTime{Time: now.UTC(), Valid: true},
			NextRunAt:        sql.NullTime{Time: now.Add(worker.RetryDelay()).UTC(), Valid: true},
			LastStatus:       sql.NullString{String: "failed", Valid: true},
			LastErrorCode:    sql.NullString{String: worker.FailureErrorCode(), Valid: true},
			LastErrorMessage: sql.NullString{String: executeErr.Error(), Valid: true},
			UpdatedAt:        now.UTC(),
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
		LastFinishedAt: sql.NullTime{Time: now.UTC(), Valid: true},
		NextRunAt:      sql.NullTime{Time: now.Add(worker.RetryDelay()).UTC(), Valid: true},
		LastStatus:     sql.NullString{String: "succeeded", Valid: true},
		UpdatedAt:      now.UTC(),
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
