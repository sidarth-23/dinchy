package jobs

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
)

// Migrate applies River's schema migrations, creating the job-queue tables when
// absent. It is idempotent and safe to run on every startup.
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Logger: logger})
	if err != nil {
		return apperrors.Annotate(err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return apperrors.Annotate(err)
	}
	return nil
}
