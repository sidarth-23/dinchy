package logging

import (
	"context"
	"log/slog"
)

type ctxKey int

const ctxKeyLogger ctxKey = iota

// WithContext stores a request-scoped logger in the context.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyLogger, logger)
}

// FromContext returns the request-scoped logger from the context, or the
// process default logger if none has been attached.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(ctxKeyLogger).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}
