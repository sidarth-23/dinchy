// Package tasks provides a durable scheduled task runtime backed by the store.
package tasks

import (
	"context"
	"log"
	"time"

	"github.com/sidarth-23/dinchy/internal/platform/clock"
)

// Runtime manages periodic background tasks with lease-based execution.
type Runtime struct {
	store  Store
	clock  clock.Clock
	owner  string
	cancel context.CancelFunc
}

// NewRuntime creates a task runtime with the given store and clock.
func NewRuntime(s Store, clk clock.Clock) *Runtime {
	return &Runtime{store: s, clock: clk, owner: "local"}
}

// Start registers built-in tasks and begins the background ticker loop.
func (r *Runtime) Start(ctx context.Context) error {
	now := r.clock.Now()
	if err := r.store.EnsureTask(ctx, "session_cleanup", 300, now); err != nil {
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
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	if err := r.runSessionCleanup(ctx); err != nil {
		log.Printf("session_cleanup run failed: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := r.runSessionCleanup(ctx); err != nil {
				log.Printf("session_cleanup run failed: %v", err)
			}
		}
	}
}

func (r *Runtime) runSessionCleanup(ctx context.Context) error {
	now := r.clock.Now()
	ok, err := r.store.ClaimTask(ctx, "session_cleanup", r.owner, now.Add(15*time.Second), now)
	if err != nil || !ok {
		return err
	}
	count, runErr := r.store.DeleteEndedSessionsOlderThan(ctx, now.Add(-24*time.Hour))
	if runErr != nil {
		if finishErr := r.store.FinishTask(ctx, "session_cleanup", now, false, "task.session_cleanup_failed", runErr.Error(), now.Add(5*time.Minute)); finishErr != nil {
			log.Printf("session_cleanup failed to persist failure state: %v", finishErr)
		}
		return runErr
	}
	log.Printf("session_cleanup deleted=%d", count)
	return r.store.FinishTask(ctx, "session_cleanup", now, true, "", "", now.Add(5*time.Minute))
}
