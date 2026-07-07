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
)

type fakePinger struct {
	err error
}

func (p fakePinger) PingContext(context.Context) error {
	return p.err
}

func newTestHandler(t *testing.T, db Pinger) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("Dinchy Internal API", "0.1.0"))
	Register(api, db)
	return r
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
