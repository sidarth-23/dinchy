package transport_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	transport "github.com/sidarth-23/dinchy/internal/transport"
)

var fixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func newTestServer(t *testing.T, devMode bool, devProxyURL string) http.Handler {
	t.Helper()
	sessionSvc := session.NewService(nil, id.NewGenerator(), clock.Fixed(fixedTime), config.DefaultSession(), config.DefaultCache(), nil)
	svc, err := auth.NewService(nil, nil, sessionSvc, id.NewGenerator(), clock.Fixed(fixedTime), config.DefaultAuth(), nil, nil, cache.NewKeyer("test"), nil, nil)
	require.NoError(t, err)
	dist := fstest.MapFS{"hello.txt": {Data: []byte("hello")}}
	srv := transport.New(":0", dist, svc, sessionSvc, nil, nil, false, devMode, devProxyURL, nil)
	return srv.Handler
}

func doRequest(t *testing.T, handler http.Handler, method, path string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, "http://example.test"+path, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Result()
}

func TestNew_ServesFrontend(t *testing.T) {
	t.Parallel()
	handler := newTestServer(t, false, "")

	resp := doRequest(t, handler, http.MethodGet, "/hello.txt")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(body))
}

func TestNew_DevMode_InvalidProxyFallsBackTo404(t *testing.T) {
	t.Parallel()
	handler := newTestServer(t, true, "://bad-url")

	resp := doRequest(t, handler, http.MethodGet, "/hello.txt")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
