package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sidarth-23/dinchy/internal/transport/middleware"
)

func TestNoStore_SetsCacheControl(t *testing.T) {
	t.Parallel()

	var sawNext bool
	handler := middleware.NoStore()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawNext = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.test/api/thing", http.NoBody)
	handler.ServeHTTP(rr, req)

	assert.True(t, sawNext)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
}
