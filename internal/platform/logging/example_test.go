package logging_test

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

// ExampleInfo records a completed milestone with stable structured fields.
// Logger output carries a timestamp, so this example has no Output block.
func ExampleInfo() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	logging.Info(context.Background(), logger, "worker run completed",
		slog.String("task", "purge-sessions"),
		slog.Duration("duration", 42*time.Millisecond))
}

// ExampleWarn records a recoverable fallback that does not fail the operation.
func ExampleWarn() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	logging.Warn(context.Background(), logger, "no workers are registered")
}
