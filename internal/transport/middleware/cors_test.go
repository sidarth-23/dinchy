package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/transport/middleware"
)

func TestCORS_AllowsSameOrigin(t *testing.T) {
	t.Parallel()

	called := false
	handler := middleware.CORS("https")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "https://app.example.test/api/bootstrap", http.NoBody)
	req.Header.Set("Origin", "https://app.example.test")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.True(t, called)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "https://app.example.test", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
	assert.Contains(t, rr.Header().Get("Vary"), "Origin")
}

func TestCORS_RejectsForeignOrigin(t *testing.T) {
	t.Parallel()

	handler := middleware.CORS("https")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unexpected handler execution")
	}))

	req := httptest.NewRequest(http.MethodPost, "https://app.example.test/api/auth/login", http.NoBody)
	req.Header.Set("Origin", "https://evil.example.test")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

// TestCORS_AllowsSameOriginPreflight covers the only preflight that should ever succeed.
// The Vite dev server is deliberately not an allowed origin: Caddy forwards to it on the
// panel's own hostname, so in development the browser is same-origin too.
func TestCORS_AllowsSameOriginPreflight(t *testing.T) {
	t.Parallel()

	called := false
	handler := middleware.CORS("https")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "https://app.example.test/api/auth/login", http.NoBody)
	req.Header.Set("Origin", "https://app.example.test")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-csrf-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.False(t, called, "a preflight is answered by the middleware, not the handler")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "https://app.example.test", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "POST", rr.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, X-Csrf-Token", rr.Header().Get("Access-Control-Allow-Headers"))
	vary := strings.Join(rr.Header()["Vary"], ",")
	assert.Contains(t, vary, "Origin")
	assert.Contains(t, vary, "Access-Control-Request-Method")
	assert.Contains(t, vary, "Access-Control-Request-Headers")
}

func TestCORS_RejectsTheViteDevServerOrigin(t *testing.T) {
	t.Parallel()

	handler := middleware.CORS("https")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unexpected handler execution")
	}))

	req := httptest.NewRequest(http.MethodPost, "https://app.example.test/api/auth/login", http.NoBody)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// TestCORS_MatchesHostIncludingPort guards the non-default-port case: Caddy forwards the
// original Host with its port, and the browser's Origin carries it too.
func TestCORS_MatchesHostIncludingPort(t *testing.T) {
	t.Parallel()

	called := false
	handler := middleware.CORS("https")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "https://localhost:8443/api/auth/login", http.NoBody)
	req.Header.Set("Origin", "https://localhost:8443")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rr.Code)
}
