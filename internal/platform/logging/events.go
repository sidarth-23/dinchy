package logging

import (
	"context"
	"log/slog"
)

// Info records an informational application event at the owning boundary.
func Info(ctx context.Context, logger *slog.Logger, message string, attrs ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.InfoContext(ctx, message, attrs...)
}

// Warn records a recoverable application warning at the owning boundary.
func Warn(ctx context.Context, logger *slog.Logger, message string, attrs ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.WarnContext(ctx, message, attrs...)
}
