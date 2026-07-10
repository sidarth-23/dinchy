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
	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	transport "github.com/sidarth-23/dinchy/internal/transport"
)

var fixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func newTestServer(t *testing.T, devMode bool, devProxyURL string) http.Handler {
	t.Helper()
	base := features.ServiceDependencies{Clock: clock.Fixed(fixedTime), IDGenerator: id.NewGenerator()}
	sessionSvc, err := session.NewService(session.Dependencies{Base: base, Config: config.DefaultSession(), CacheConfig: config.DefaultCache()})
	require.NoError(t, err)
	svc, err := auth.NewService(auth.Dependencies{Base: base, Sessions: sessionSvc, AuthConfig: config.DefaultAuth(), CacheKeyer: cache.NewKeyer("test")})
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
