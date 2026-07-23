package store

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

func TestQueryTracer_LogsQueryWithoutArgs(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: logging.LevelTrace}))
	tracer := queryTracer{logger: logger}

	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{
		SQL:  "SELECT * FROM users WHERE email = $1",
		Args: []any{"secret@example.com"},
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})

	output := buffer.String()
	assert.Contains(t, output, "Database query completed")
	assert.Contains(t, output, "SELECT * FROM users WHERE email = $1")
	assert.Contains(t, output, "SELECT 1")
	assert.False(t, strings.Contains(output, "secret@example.com"))
}

func TestQueryTracer_SuppressedAtDebugLevel(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tracer := queryTracer{logger: logger}

	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{
		SQL: "SELECT 1",
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})

	assert.Empty(t, strings.TrimSpace(buffer.String()))
}
