package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/transport/middleware"
)

func TestRecover_PanicReturnsJSONErrorAndStructuredLog(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	handler := middleware.Recover(logger, apperrors.NewRenderer(i18n.Default, false))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	recorder := httptest.NewRecorder()
	require.NotPanics(t, func() {
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://example.test/panic", http.NoBody))
	})

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.Equal(t, "server.internal_error", decodeErrorCode(t, recorder.Body.Bytes()))

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Len(t, lines, 1)

	record := decodeRecord(t, lines[0])
	require.Equal(t, "ERROR", record["level"])
	require.Equal(t, "Recovered handler panic", record["msg"])
	require.Equal(t, "boom", record["panic"])
	require.NotEmpty(t, record["stack"])
}

func TestRecover_HTTPErrAbortHandlerIsRepanicked(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	handler := middleware.Recover(logger, apperrors.NewRenderer(i18n.Default, false))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	recorder := httptest.NewRecorder()
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://example.test/abort", http.NoBody))
	})
	require.Empty(t, strings.TrimSpace(buffer.String()))
}

func TestRecover_PanicAfterWriteDoesNotWriteErrorResponse(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	handler := middleware.Recover(logger, apperrors.NewRenderer(i18n.Default, false))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
		panic("boom")
	}))

	recorder := httptest.NewRecorder()
	require.NotPanics(t, func() {
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "http://example.test/started", http.NoBody))
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "hello", recorder.Body.String())

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Len(t, lines, 1)
	record := decodeRecord(t, lines[0])
	require.Equal(t, "boom", record["panic"])
	require.NotEmpty(t, record["stack"])
}
