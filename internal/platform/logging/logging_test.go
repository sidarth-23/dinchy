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

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

func TestRedactedValue_MasksWhenVisibilityDisabled(t *testing.T) {
	t.Cleanup(func() {
		logging.SetRedactionVisible(false)
	})
	logging.SetRedactionVisible(false)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	logger.Info("masked", slog.Any("secret", logging.Redacted(map[string]any{"token": "value"})))

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

	logger.Info("revealed", slog.Any("secret", logging.Redacted(map[string]any{"token": "value"})))

	record := decodeLogRecord(t, buffer.String())
	require.Equal(t, map[string]any{"token": "value"}, record["secret"])
}

func TestRedactedValue_RevealsNilPointerAsNull(t *testing.T) {
	t.Cleanup(func() {
		logging.SetRedactionVisible(false)
	})
	logging.SetRedactionVisible(true)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	logger.Info("nil", slog.Any("secret", logging.Redacted((*string)(nil))))

	record := decodeLogRecord(t, buffer.String())
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

	logging.Error(context.Background(), logger, "request failed", apperrors.BadRequest(i18n.Msg(i18n.CodeTransportRequestValidationFailed)))

	require.Empty(t, strings.TrimSpace(buffer.String()))
}

func TestError_LogsInternalErrors(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	logging.Error(context.Background(), logger, "request failed", apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(errors.New("boom"))))

	record := decodeLogRecord(t, buffer.String())
	require.Equal(t, "request failed", record["msg"])
	require.Equal(t, "platform.server.internal_error", record["code"])
	require.Equal(t, float64(500), record["status"])
	require.Equal(t, "boom", record["cause"])
}

func TestError_LogsOnlyOnceForSameAppError(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	err := apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(errors.New("boom")))
	logging.Error(context.Background(), logger, "request failed", err)
	logging.Error(context.Background(), logger, "request failed", err)

	require.Len(t, strings.Split(strings.TrimSpace(buffer.String()), "\n"), 1)
	require.True(t, err.Logged())
}

func TestError_DoesNotMarkSuppressedClientErrors(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	err := apperrors.BadRequest(i18n.Msg(i18n.CodeTransportRequestValidationFailed))
	logging.Error(context.Background(), logger, "request failed", err)

	require.Empty(t, strings.TrimSpace(buffer.String()))
	require.False(t, err.Logged())
}

func TestInfoAndWarn_UseSharedHelpers(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	logging.Info(context.Background(), logger, "startup complete", "component", "transport")
	logging.Warn(context.Background(), logger, "dev proxy disabled", "component", "transport")

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Len(t, lines, 2)
	info := decodeLogRecord(t, lines[0])
	warn := decodeLogRecord(t, lines[1])
	require.Equal(t, "startup complete", info["msg"])
	require.Equal(t, "dev proxy disabled", warn["msg"])
}

func decodeLogRecord(t *testing.T, raw string) map[string]any {
	t.Helper()
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(raw)), &record))
	return record
}
