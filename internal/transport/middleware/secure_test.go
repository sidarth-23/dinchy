package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sidarth-23/dinchy/internal/transport/middleware"
)

func secureHeadersResponse(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	handler := middleware.SecureHeaders()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://example.test/api/bootstrap", http.NoBody))
	return rr
}

func TestSecureHeaders_AppliesBaselineHeaders(t *testing.T) {
	t.Parallel()

	var sawNext bool
	handler := middleware.SecureHeaders()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sawNext = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://example.test/api/bootstrap", http.NoBody))

	assert.True(t, sawNext)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rr.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", rr.Header().Get("Referrer-Policy"))
}

// TestSecureHeaders_CSPGrantsNothing pins that the policy is an API policy. Dinchy serves
// no documents — Caddy delivers the web UI — so a JSON response has no legitimate reason
// to load a script, a style, or a frame, and the policy should grant none of it.
func TestSecureHeaders_CSPGrantsNothing(t *testing.T) {
	t.Parallel()

	csp := secureHeadersResponse(t).Header().Get("Content-Security-Policy")

	assert.Equal(t, "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'", csp)
	for _, directive := range []string{"script-src", "style-src", "connect-src", "img-src", "font-src"} {
		assert.NotContains(t, csp, directive,
			"%s belongs to the document policy Caddy sets, not to an API response", directive)
	}
	assert.NotContains(t, csp, "'self'", "an API response grants no origin, not even its own")
	assert.NotContains(t, csp, "unsafe-inline", "the dev relaxations Vite needs are Vite's to declare")
}

// TestSecureHeaders_OmitsHSTS records that Caddy owns it: the responses that most need
// HSTS are Caddy's own redirect and error pages, which never reach this handler.
func TestSecureHeaders_OmitsHSTS(t *testing.T) {
	t.Parallel()

	assert.Empty(t, secureHeadersResponse(t).Header().Get("Strict-Transport-Security"))
}
