package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/transport/middleware"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

func TestCORS_AllowsSameOrigin(t *testing.T) {
	t.Parallel()

	called := false
	handler := middleware.CORS(false, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "https://app.example.test/api/bootstrap", nil)
	req = req.WithContext(support.WithSecure(req.Context(), true))
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

	handler := middleware.CORS(false, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unexpected handler execution")
	}))

	req := httptest.NewRequest(http.MethodPost, "https://app.example.test/api/auth/login", nil)
	req = req.WithContext(support.WithSecure(req.Context(), true))
	req.Header.Set("Origin", "https://evil.example.test")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_AllowsDevOriginPreflight(t *testing.T) {
	t.Parallel()

	called := false
	handler := middleware.CORS(true, "http://127.0.0.1:5173")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "https://app.example.test/api/auth/login", nil)
	req = req.WithContext(support.WithSecure(req.Context(), true))
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-csrf-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "http://127.0.0.1:5173", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "POST", rr.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, X-Csrf-Token", rr.Header().Get("Access-Control-Allow-Headers"))
	vary := strings.Join(rr.Header()["Vary"], ",")
	assert.Contains(t, vary, "Origin")
	assert.Contains(t, vary, "Access-Control-Request-Method")
	assert.Contains(t, vary, "Access-Control-Request-Headers")
}
