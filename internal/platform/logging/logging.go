// Package logging configures structured application logging.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"

	"github.com/sidarth-23/dinchy/internal/config"
)

var revealRedacted atomic.Bool

func New(cfg config.LoggingConfig, revealSensitive bool, otel slog.Handler) *slog.Logger {
	SetRedactionVisible(revealSensitive)

	level := slog.LevelInfo
	switch cfg.Level {
	case config.LogLevelDebug:
		level = slog.LevelDebug
	case config.LogLevelWarn:
		level = slog.LevelWarn
	case config.LogLevelError:
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level, AddSource: cfg.AddSource}
	var base slog.Handler
	if cfg.Format == config.LogFormatText {
		base = slog.NewTextHandler(os.Stdout, opts)
	} else {
		base = slog.NewJSONHandler(os.Stdout, opts)
	}
	base = traceHandler{next: base}
	if otel != nil {
		base = multiHandler{handlers: []slog.Handler{base, traceHandler{next: otel}}}
	}
	return slog.New(base)
}

func Default() *slog.Logger {
	return slog.Default()
}

// SetRedactionVisible controls whether redacted log values render their
// underlying value instead of a placeholder.
func SetRedactionVisible(visible bool) {
	revealRedacted.Store(visible)
}

func CloseAll(ctx context.Context, closers ...io.Closer) error {
	var err error
	for _, closer := range closers {
		if closer == nil {
			continue
		}
		if closeErr := closer.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	_ = ctx
	return err
}

type multiHandler struct {
	handlers []slog.Handler
}

func (h multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func (h multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		out = append(out, handler.WithAttrs(attrs))
	}
	return multiHandler{handlers: out}
}

func (h multiHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		out = append(out, handler.WithGroup(name))
	}
	return multiHandler{handlers: out}
}

type traceHandler struct {
	next slog.Handler
}

func (h traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h traceHandler) Handle(ctx context.Context, record slog.Record) error {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, record)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{next: h.next.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{next: h.next.WithGroup(name)}
}
