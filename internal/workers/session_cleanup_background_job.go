package workers

import (
	"context"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

const (
	sessionCleanupJobName           = "session.cleanup"
	sessionCleanupInterval          = 5 * time.Minute
	sessionCleanupRetentionDuration = 24 * time.Hour
)

// SessionCleanupStore is the data access the session cleanup job requires.
type SessionCleanupStore interface {
	DeleteEndedSessionsOlderThan(ctx context.Context, arg sqlcgen.DeleteEndedSessionsOlderThanParams) (pgconn.CommandTag, error)
}

// RegisterSessionCleanup schedules the recurring job that prunes ended sessions
// past the retention window. The first run happens at startup, then every interval.
func RegisterSessionCleanup(sched gocron.Scheduler, store SessionCleanupStore, clk clock.Clock) error {
	_, err := sched.NewJob(
		gocron.DurationJob(sessionCleanupInterval),
		gocron.NewTask(func(ctx context.Context) error {
			return cleanupEndedSessions(ctx, store, clk)
		}),
		gocron.WithName(sessionCleanupJobName),
		gocron.WithStartAt(gocron.WithStartImmediately()),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsWorkersSessionCleanup), apperrors.WithCause(err))
	}
	return nil
}

func cleanupEndedSessions(ctx context.Context, store SessionCleanupStore, clk clock.Clock) error {
	now := clk.Now()
	if _, err := store.DeleteEndedSessionsOlderThan(ctx, sqlcgen.DeleteEndedSessionsOlderThanParams{
		ExpiresAt: sqltype.Timestamptz(now.Add(-sessionCleanupRetentionDuration)),
		UpdatedAt: sqltype.Timestamptz(now),
	}); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsWorkersSessionCleanup), apperrors.WithCause(err))
	}
	return nil
}
