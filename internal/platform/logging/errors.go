package logging

import (
	"context"
	stdErrors "errors"
	"log/slog"
	"net/http"
	"strings"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
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
		if appErr.Logged() {
			return
		}
		if appErr.Status() < http.StatusInternalServerError {
			return
		}
		attrs = append(
			attrs,
			slog.Int("status", appErr.Status()),
			slog.String("code", string(appErr.Code())),
		)
		attrs = append(attrs, codePathAttrs(string(appErr.Code()))...)
		if meta := appErr.Meta(); len(meta) > 0 {
			attrs = append(attrs, slog.Any("meta", meta))
		}
		if cause := stdErrors.Unwrap(appErr); cause != nil {
			attrs = append(attrs, slog.Any("cause", cause))
		}
		appErr.MarkLogged()
		logger.ErrorContext(ctx, message, attrs...)
		return
	}

	attrs = append(attrs, slog.Any("error", err))
	logger.ErrorContext(ctx, message, attrs...)
}

// codePathAttrs derives structured module/area/operation fields from a dotted
// error code (e.g. auth.login.find_user), preserving the observability that
// flow/stage metadata used to carry at the call site.
func codePathAttrs(code string) []any {
	segments := strings.Split(code, ".")
	if len(segments) == 0 || segments[0] == "" {
		return nil
	}
	attrs := []any{slog.String("module", segments[0])}
	switch len(segments) {
	case 1:
		return attrs
	case 2:
		return append(attrs, slog.String("operation", segments[1]))
	default:
		return append(
			attrs,
			slog.String("area", segments[1]),
			slog.String("operation", strings.Join(segments[2:], ".")),
		)
	}
}

// Panic records a recovered panic once at the boundary where it is handled.
// slog has no panic level; this logs at Error level with the panic value and stack.
func Panic(ctx context.Context, logger *slog.Logger, message string, recovered any, stack []byte, attrs ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	attrs = append(
		attrs,
		slog.Any("panic", recovered),
		slog.String("stack", string(stack)),
	)
	logger.ErrorContext(ctx, message, attrs...)
}
