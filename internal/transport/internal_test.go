package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transport "github.com/sidarth-23/dinchy/internal/transport"
)

type fakePinger struct {
	err error
}

func (p fakePinger) PingContext(context.Context) error {
	return p.err
}

func TestNewInternal_Healthz(t *testing.T) {
	t.Parallel()
	srv := transport.NewInternal(":0", fakePinger{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "ok", rr.Body.String())
}

func TestNewInternal_Readyz_Healthy(t *testing.T) {
	t.Parallel()
	srv := transport.NewInternal(":0", fakePinger{})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, true, body["ready"])
}

func TestNewInternal_Readyz_Unhealthy(t *testing.T) {
	t.Parallel()
	srv := transport.NewInternal(":0", fakePinger{err: assert.AnError})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, false, body["ready"])
}
