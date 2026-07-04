package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

func TestRedactedValue_MasksWhenVisibilityDisabled(t *testing.T) {
	t.Cleanup(func() {
		logging.SetRedactionVisible(false)
	})
	logging.SetRedactionVisible(false)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	logger.Info("masked", slog.Any("secret", logging.Redacted(map[string]any{
		"token":  "abc123",
		"nested": map[string]any{"password": "hunter2"},
	})))

	record := decodeLogRecord(t, buffer.String())
	require.Equal(t, "masked", record["msg"])
	require.Equal(t, "[redacted]", record["secret"])
}

func TestRedactedValue_RevealsWhenVisibilityEnabled(t *testing.T) {
	t.Cleanup(func() {
		logging.SetRedactionVisible(false)
	})
	logging.SetRedactionVisible(true)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	logger.Info("revealed", slog.Any("secret", logging.Redacted(map[string]any{
		"token":  "abc123",
		"nested": map[string]any{"password": "hunter2"},
	})))

	record := decodeLogRecord(t, buffer.String())
	require.Equal(t, "revealed", record["msg"])

	secret, ok := record["secret"].(map[string]any)
	require.True(t, ok, "expected revealed redacted value to stay structured")
	require.Equal(t, "abc123", secret["token"])
}

func TestRedactedValue_RevealsNilPointerAsNull(t *testing.T) {
	t.Cleanup(func() {
		logging.SetRedactionVisible(false)
	})
	logging.SetRedactionVisible(true)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	var secret *string
	logger.Info("revealed", slog.Any("secret", logging.Redacted(secret)))

	record := decodeLogRecord(t, buffer.String())
	require.Equal(t, "revealed", record["msg"])
	require.Nil(t, record["secret"])
}

func TestLoggerFromContext_ReturnsAttachedLogger(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	ctx := logging.WithLogger(context.Background(), logger)

	require.Same(t, logger, logging.LoggerFromContext(ctx))
}

func TestError_SkipsClientErrors(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	logging.Error(context.Background(), logger, "request failed", apperrors.BadRequest(i18n.Msg(i18n.CodeRequestValidationFailed)))

	require.Empty(t, strings.TrimSpace(buffer.String()))
}

func TestError_LogsInternalErrors(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	logging.Error(context.Background(), logger, "request failed", apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(errors.New("boom"))))

	record := decodeLogRecord(t, buffer.String())
	require.Equal(t, "request failed", record["msg"])
	require.Equal(t, "server.internal_error", record["code"])
	require.Equal(t, float64(500), record["status"])
	require.Equal(t, "boom", record["cause"])
}

func TestInfoAndWarn_UseSharedHelpers(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	logging.Info(context.Background(), logger, "Application started", slog.String("component", "app"))
	logging.Warn(context.Background(), logger, "Invalid dev proxy URL", slog.String("component", "transport"))

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Len(t, lines, 2)

	info := decodeLogRecord(t, lines[0])
	warn := decodeLogRecord(t, lines[1])

	require.Equal(t, "Application started", info["msg"])
	require.Equal(t, "Invalid dev proxy URL", warn["msg"])
	require.Equal(t, "app", info["component"])
	require.Equal(t, "transport", warn["component"])
}

func decodeLogRecord(t *testing.T, raw string) map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(raw), "\n")
	require.Len(t, lines, 1)

	record := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
	return record
}
