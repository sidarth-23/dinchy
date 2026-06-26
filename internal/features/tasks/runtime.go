// Package tasks provides a durable scheduled task runtime backed by the store.
package tasks

import (
	"context"
	"errors"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
)

// Runtime manages periodic background tasks with lease-based execution.
type Runtime struct {
	store  Store
	clock  clock.Clock
	owner  string
	cancel context.CancelFunc
	errCh  chan<- error
}

// NewRuntime creates a task runtime with the given store and clock.
func NewRuntime(s Store, clk clock.Clock, errCh chan<- error) *Runtime {
	return &Runtime{store: s, clock: clk, owner: "local", errCh: errCh}
}

// Start registers built-in tasks and begins the background ticker loop.
func (r *Runtime) Start(ctx context.Context) error {
	now := r.clock.Now()
	if err := r.store.EnsureTask(ctx, "session_cleanup", 300, now); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.TaskSessionCleanup),
			apperrors.WithStage(apperrors.StageEnsureTask),
		)
	}
	cctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	go r.loop(cctx)
	return nil
}

// Stop cancels the background loop.
func (r *Runtime) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Runtime) loop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	if err := r.runSessionCleanup(ctx); err != nil {
		r.report(err)
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := r.runSessionCleanup(ctx); err != nil {
				r.report(err)
				return
			}
		}
	}
}

func (r *Runtime) runSessionCleanup(ctx context.Context) error {
	now := r.clock.Now()
	ok, err := r.store.ClaimTask(ctx, "session_cleanup", r.owner, now.Add(15*time.Second), now)
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.TaskSessionCleanup),
			apperrors.WithStage(apperrors.StageClaimTask),
		)
	}
	if !ok {
		return nil
	}
	count, runErr := r.store.DeleteEndedSessionsOlderThan(ctx, now.Add(-24*time.Hour))
	if runErr != nil {
		if finishErr := r.store.FinishTask(ctx, "session_cleanup", now, false, "task.session_cleanup_failed", runErr.Error(), now.Add(5*time.Minute)); finishErr != nil {
			return errors.Join(
				apperrors.Annotate(runErr,
					apperrors.WithTask(apperrors.TaskSessionCleanup),
					apperrors.WithStage(apperrors.StageDeleteEndedSessions),
				),
				apperrors.Annotate(finishErr,
					apperrors.WithTask(apperrors.TaskSessionCleanup),
					apperrors.WithStage(apperrors.StageFinishFailedRun),
				),
			)
		}
		return apperrors.Annotate(runErr,
			apperrors.WithTask(apperrors.TaskSessionCleanup),
			apperrors.WithStage(apperrors.StageDeleteEndedSessions),
		)
	}
	if err := r.store.FinishTask(ctx, "session_cleanup", now, true, "", "", now.Add(5*time.Minute)); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.TaskSessionCleanup),
			apperrors.WithStage(apperrors.StageFinishSuccess),
			apperrors.WithDeletedCount(apperrors.DeletedCount(count)),
		)
	}
	return nil
}

func (r *Runtime) report(err error) {
	if r.errCh == nil || err == nil {
		return
	}
	r.errCh <- err
}
