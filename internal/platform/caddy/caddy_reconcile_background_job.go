package caddy

import (
	"context"

	"github.com/go-co-op/gocron/v2"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

const caddyReconcileJobName = "caddy.reconcile"

// RegisterReconcileWorker schedules the recurring job that re-pushes the desired
// configuration to Caddy.
//
// It repairs the three ways Caddy can end up out of step without anyone noticing: the
// startup reconcile failed because Caddy was not listening yet, Caddy restarted and
// resumed an older configuration, or someone reconfigured it out of band. The job does
// not start immediately — the caller already reconciles once during startup.
//
// The error is returned, not logged: the scheduler's own error listener is the single
// place worker failures are recorded.
func RegisterReconcileWorker(sched gocron.Scheduler, reconciler *Reconciler) error {
	if reconciler == nil || !reconciler.cfg.Enabled {
		return nil
	}
	_, err := sched.NewJob(
		gocron.DurationJob(reconciler.cfg.ReconcileInterval),
		gocron.NewTask(func(ctx context.Context) error {
			if _, err := reconciler.ReconcileAll(ctx); err != nil {
				return apperrors.Annotate(err, apperrors.WithTask(caddyReconcileJobName))
			}
			return nil
		}),
		gocron.WithName(caddyReconcileJobName),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return apperrors.Internal(
			i18n.Msg(i18n.CodeDiagnosticsWorkersCaddyReconcile),
			apperrors.WithTask(caddyReconcileJobName),
			apperrors.WithCause(err),
		)
	}
	return nil
}
