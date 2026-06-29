package transport_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite"
	transport "github.com/sidarth-23/dinchy/internal/transport"
)

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time {
	return c.now
}

var fixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func newTestServer(t *testing.T, devMode bool, devProxyURL string) http.Handler {
	t.Helper()
	db := core.OpenTestDB(t, sqlite.Open, filepath.Join(t.TempDir(), "test.db"))
	svc := auth.NewService(db, id.NewGenerator(), fakeClock{now: fixedTime})
	dist := fstest.MapFS{"hello.txt": {Data: []byte("hello")}}
	srv := transport.New(":0", dist, svc, db, false, devMode, devProxyURL)
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
