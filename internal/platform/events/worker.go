package events

import (
	"context"

	"github.com/go-co-op/gocron/v2"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// RegisterWorkers schedules one recurring job per registered subscriber that
// drains its pending events. Each job runs at the configured worker interval,
// starts immediately, and never overlaps its own previous run.
func RegisterWorkers(sched gocron.Scheduler, svc *Service) error {
	if svc == nil {
		return nil
	}
	for _, name := range svc.SubscriberNames() {
		if _, err := sched.NewJob(
			gocron.DurationJob(svc.cfg.WorkerInterval),
			gocron.NewTask(func(ctx context.Context) error {
				_, err := svc.ProcessSubscriber(ctx, name)
				return err
			}),
			gocron.WithName("eventbus."+name),
			gocron.WithStartAt(gocron.WithStartImmediately()),
			gocron.WithSingletonMode(gocron.LimitModeReschedule),
		); err != nil {
			return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsWorkersEventProcessing), apperrors.WithCause(err))
		}
	}
	return nil
}
