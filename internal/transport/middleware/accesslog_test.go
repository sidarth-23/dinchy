package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/platform/logging"
	"github.com/sidarth-23/dinchy/internal/transport/middleware"
)

func TestAccessLog_AttachesRequestLogger(t *testing.T) {
	t.Cleanup(func() {
		logging.SetRedactionVisible(false)
	})
	logging.SetRedactionVisible(false)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	handler := chimw.RequestID(middleware.AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logging.LoggerFromContext(r.Context()).InfoContext(
			r.Context(), "inside handler",
			slog.Any("secret", logging.Redacted("sensitive-value")),
		)
		w.WriteHeader(http.StatusCreated)
	})))

	request := httptest.NewRequest(http.MethodPost, "https://app.example.test/api/auth/login", http.NoBody)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Len(t, lines, 2)

	first := decodeRecord(t, lines[0])
	second := decodeRecord(t, lines[1])

	require.Equal(t, "inside handler", first["msg"])
	require.Equal(t, "[redacted]", first["secret"])
	require.Equal(t, "HTTP request completed", second["msg"])
	require.Equal(t, "POST", second["method"])
	require.Equal(t, "/api/auth/login", second["path"])
	require.Equal(t, float64(http.StatusCreated), second["status"])
	require.NotEmpty(t, first["request_id"])
	require.Equal(t, first["request_id"], second["request_id"])
}

func decodeRecord(t *testing.T, raw string) map[string]any {
	t.Helper()

	record := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(raw), &record))
	return record
}
