// Package workers constructs the background job scheduler and registers the
// recurring jobs the application owns.
package workers

import (
	"context"
	"log/slog"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

// New builds the background job scheduler with a bounded concurrency pool, a
// graceful-shutdown drain window, an slog-backed logger, and a global listener
// that logs every job failure once at this boundary.
func New(logger *slog.Logger, cfg config.WorkerConfig) (gocron.Scheduler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	scheduler, err := gocron.NewScheduler(
		gocron.WithLimitConcurrentJobs(uint(cfg.Concurrency), gocron.LimitModeWait),
		gocron.WithStopTimeout(cfg.ShutdownTimeout),
		gocron.WithLogger(newSlogLogger(logger)),
		gocron.WithGlobalJobOptions(
			gocron.WithEventListeners(
				gocron.AfterJobRunsWithError(func(_ uuid.UUID, name string, err error) {
					logging.Error(context.Background(), logger, "Background job failed", err,
						slog.String("job", name))
				}),
			),
		),
	)
	if err != nil {
		return nil, apperrors.Annotate(err)
	}
	return scheduler, nil
}

// slogLogger adapts gocron's framework logger to the application's slog logger.
// It carries gocron's own diagnostics; job failures are logged via the scheduler
// error listener in New, not here.
type slogLogger struct{ logger *slog.Logger }

func newSlogLogger(logger *slog.Logger) gocron.Logger { return slogLogger{logger: logger} }

// Debug logs a gocron debug message.
func (l slogLogger) Debug(msg string, args ...any) { l.logger.Debug(msg, args...) }

// Info logs a gocron info message.
func (l slogLogger) Info(msg string, args ...any) { l.logger.Info(msg, args...) }

// Warn logs a gocron warning message.
func (l slogLogger) Warn(msg string, args ...any) { l.logger.Warn(msg, args...) }

// Error logs a gocron error message.
func (l slogLogger) Error(msg string, args ...any) { l.logger.Error(msg, args...) }
