package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsoleHandler_RendersBlockAttrAsIndentedLines(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(newConsoleHandler(&buffer, slog.LevelDebug, false, map[string]bool{"query": true}))

	logger.InfoContext(context.Background(), "Database query completed",
		slog.String("component", "store"),
		slog.String("query", "SELECT id\nFROM river_job\nWHERE state = 'available'"),
		slog.String("command_tag", "UPDATE 0"),
	)

	out := buffer.String()
	require.NotContains(t, out, `\n`, "block attribute must not be escaped onto one line")
	require.Contains(t, out, "component=store")
	require.Contains(t, out, `command_tag="UPDATE 0"`)
	require.Contains(t, out, "\n  SELECT id\n  FROM river_job\n  WHERE state = 'available'\n")
	require.NotContains(t, out, "query=", "block attribute must not render inline on the main line")
}

func TestReplaceLevelName_RendersTraceInJSON(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer,
		&slog.HandlerOptions{Level: LevelTrace, ReplaceAttr: replaceLevelName}))

	logger.Log(context.Background(), LevelTrace, "Database query completed")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buffer.Bytes(), &record))
	require.Equal(t, "TRACE", record["level"])
}

func TestReplaceLevelName_RendersTraceInConsole(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(newConsoleHandler(&buffer, LevelTrace, false, nil))

	logger.Log(context.Background(), LevelTrace, "Database query completed")

	require.Contains(t, buffer.String(), "TRACE")
}

func TestConsoleHandler_KeepsNonBlockAttrsInline(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(newConsoleHandler(&buffer, slog.LevelDebug, false, map[string]bool{"query": true}))

	logger.InfoContext(context.Background(), "plain", slog.String("component", "store"))

	out := buffer.String()
	require.Equal(t, 1, strings.Count(out, "\n"), "a record with no block attribute stays on a single line")
	require.Contains(t, out, "component=store")
}
