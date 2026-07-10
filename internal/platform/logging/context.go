package logging

import (
	"context"
	"log/slog"
)

type ctxKey int

const ctxKeyLogger ctxKey = iota

// WithLogger stores a request-scoped logger in the context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyLogger, logger)
}

// LoggerFromContext returns the request-scoped logger from the context, or the
// process default logger if none has been attached.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	return LoggerFromContextOr(ctx, slog.Default())
}

// LoggerFromContextOr returns the request-scoped logger or fallback when none is attached.
func LoggerFromContextOr(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(ctxKeyLogger).(*slog.Logger); ok && logger != nil {
			return logger
		}
	}
	if fallback != nil {
		return fallback
	}
	return slog.Default()
}
