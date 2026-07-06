package logging

import (
	"context"
	stdErrors "errors"
	"log/slog"
	"net/http"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
)

// Error records a failure once at the boundary where it is handled.
// Structured client errors are skipped; internal errors are logged with code,
// status, and preserved metadata.
func Error(ctx context.Context, logger *slog.Logger, message string, err error, attrs ...any) {
	logError(ctx, logger, message, err, attrs...)
}

// HTTPError records a request failure once for HTTP boundaries.
// Non-5xx responses are intentionally skipped so expected client errors are not
// logged multiple times across transport and service layers.
func HTTPError(ctx context.Context, logger *slog.Logger, status int, message string, err error, attrs ...any) {
	if status < http.StatusInternalServerError {
		return
	}
	logError(ctx, logger, message, err, attrs...)
}

func logError(ctx context.Context, logger *slog.Logger, message string, err error, attrs ...any) {
	if err == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	if appErr, ok := stdErrors.AsType[*apperrors.AppError](err); ok {
		if appErr.Status() < http.StatusInternalServerError {
			return
		}
		attrs = append(attrs,
			slog.Int("status", appErr.Status()),
			slog.String("code", string(appErr.Code())),
		)
		if meta := appErr.Meta(); len(meta) > 0 {
			attrs = append(attrs, slog.Any("meta", meta))
		}
		if cause := stdErrors.Unwrap(appErr); cause != nil {
			attrs = append(attrs, slog.Any("cause", cause))
		}
		logger.ErrorContext(ctx, message, attrs...)
		return
	}

	attrs = append(attrs, slog.Any("error", err))
	logger.ErrorContext(ctx, message, attrs...)
}
