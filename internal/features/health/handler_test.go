package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/foundation/clock"
)

type fakePinger struct {
	err error
}

func (p fakePinger) PingContext(context.Context) error {
	return p.err
}

func newTestHandler(t *testing.T, db Pinger, checks ...Check) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("Dinchy Internal API", "0.1.0"))
	base := (&features.Service{Clock: clock.System{}}).Named("health")
	healthAPI, err := NewAPI(base, db, checks...)
	require.NoError(t, err)
	healthAPI.Register(api)
	return r
}

// readyz issues a readiness request and decodes the payload.
func readyz(t *testing.T, handler http.Handler) (int, ReadyzBody) {
	t.Helper()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	var body ReadyzBody
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	return rr.Code, body
}

func TestAPIName(t *testing.T) {
	base := (&features.Service{Clock: clock.System{}}).Named("health")
	healthAPI, err := NewAPI(base, nil)
	require.NoError(t, err)
	assert.Equal(t, "health", healthAPI.Name())
}

func TestHealthz(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, fakePinger{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/plain")
	assert.Equal(t, "ok", rr.Body.String())
}

func TestReadyz_Healthy(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, fakePinger{})

	req := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var body ReadyzBody
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.True(t, body.Ready)
	assert.Equal(t, "dev", body.Version)
	assert.Equal(t, "ok", body.Checks["database"])
}

func TestReadyz_Unhealthy(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, fakePinger{err: assert.AnError})

	req := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	var body ReadyzBody
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.False(t, body.Ready)
	assert.Equal(t, assert.AnError.Error(), body.Checks["database"])
}

// TestReadyz_DegradedCheckStaysReady pins the contract that keeps the management plane
// reachable: a broken reverse proxy is reported, but must not make the process unready,
// because restarting Dinchy is not how a routing fault gets fixed.
func TestReadyz_DegradedCheckStaysReady(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, fakePinger{}, Check{
		Name:     "caddy",
		Degraded: func() string { return "the reverse proxy is not reachable" },
	})

	code, body := readyz(t, handler)

	assert.Equal(t, http.StatusOK, code)
	assert.True(t, body.Ready)
	assert.Equal(t, "degraded: the reverse proxy is not reachable", body.Checks["caddy"])
}

func TestReadyz_HealthyCheckIsReportedOK(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, fakePinger{}, Check{
		Name:     "caddy",
		Degraded: func() string { return "" },
	})

	code, body := readyz(t, handler)

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", body.Checks["caddy"])
}

func TestReadyz_IncompleteCheckIsIgnored(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, fakePinger{}, Check{Name: "caddy"}, Check{Degraded: func() string { return "x" }})

	code, body := readyz(t, handler)

	assert.Equal(t, http.StatusOK, code)
	assert.NotContains(t, body.Checks, "caddy")
}
