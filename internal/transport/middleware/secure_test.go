package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sidarth-23/dinchy/internal/transport/middleware"
)

func TestSecureHeaders_ProductionCSP(t *testing.T) {
	t.Parallel()

	var sawNext bool
	handler := middleware.SecureHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawNext = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", http.NoBody)
	handler.ServeHTTP(rr, req)

	assert.True(t, sawNext)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rr.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", rr.Header().Get("Referrer-Policy"))
	assert.Equal(t, "default-src 'self'; base-uri 'self'; object-src 'none'; form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'", rr.Header().Get("Content-Security-Policy"))
}

func TestSecureHeaders_DevelopmentCSP(t *testing.T) {
	t.Parallel()

	handler := middleware.SecureHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", http.NoBody)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, "default-src 'self'; base-uri 'self'; object-src 'none'; form-action 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'", rr.Header().Get("Content-Security-Policy"))
}
