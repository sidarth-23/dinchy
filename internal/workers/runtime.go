// Package workers provides durable background workers backed by the store.
package workers

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
)

// Runtime manages periodic background workers with lease-based execution.
type Runtime struct {
	store  Store
	clock  clock.Clock
	owner  string
	cancel context.CancelFunc
	errCh  chan<- error
}

// NewRuntime creates a worker runtime with the given store and clock.
func NewRuntime(s Store, clk clock.Clock, errCh chan<- error) *Runtime {
	return &Runtime{store: s, clock: clk, owner: "local", errCh: errCh}
}

// Start registers built-in workers and begins the background ticker loop.
func (r *Runtime) Start(ctx context.Context) error {
	if err := r.registerSessionCleanup(ctx); err != nil {
		return err
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
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	if err := r.runSessionCleanup(ctx); err != nil {
		r.report(err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.runSessionCleanup(ctx); err != nil {
				r.report(err)
				return
			}
		}
	}
}

func (r *Runtime) report(err error) {
	if r.errCh == nil || err == nil {
		return
	}
	r.errCh <- err
}

func (r *Runtime) registerSessionCleanup(ctx context.Context) error {
	now := r.clock.Now()
	if err := r.store.EnsureTask(ctx, sessionCleanupTaskName, sessionCleanupIntervalSeconds, now); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithTask(apperrors.TaskSessionCleanup),
			apperrors.WithStage(apperrors.StageEnsureTask),
		)
	}
	return nil
}
