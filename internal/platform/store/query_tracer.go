package store

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

type queryTraceStartKey struct{}

type queryTraceStart struct {
	sql       string
	startedAt time.Time
}

type queryTracer struct {
	logger *slog.Logger
}

func (t queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if t.logger == nil || !t.logger.Enabled(ctx, slog.LevelDebug) {
		return ctx
	}
	return context.WithValue(ctx, queryTraceStartKey{}, queryTraceStart{sql: data.SQL, startedAt: time.Now()})
}

func (t queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if t.logger == nil || !t.logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	start, _ := ctx.Value(queryTraceStartKey{}).(queryTraceStart)
	attrs := []any{
		slog.String("component", "store"),
		slog.String("query", start.sql),
		slog.String("command_tag", data.CommandTag.String()),
	}
	if !start.startedAt.IsZero() {
		attrs = append(attrs, slog.Duration("duration", time.Since(start.startedAt)))
	}
	if data.Err != nil {
		attrs = append(attrs, slog.Any("error", data.Err))
		t.logger.DebugContext(ctx, "Database query failed", attrs...)
		return
	}
	t.logger.DebugContext(ctx, "Database query completed", attrs...)
}
