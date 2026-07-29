package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sidarth-23/dinchy/internal/foundation/requestmeta"
	mw "github.com/sidarth-23/dinchy/internal/transport/middleware"
)

// observedIP runs a request through RequestInfo and reports the address it recorded.
func observedIP(remoteAddr string, headers map[string]string) string {
	var recorded string
	handler := mw.RequestInfo()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		recorded = requestmeta.RemoteIPFrom(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.RemoteAddr = remoteAddr
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return recorded
}

// TestRequestInfo_RecordsForwardedClientAddress is the regression guard for durable data:
// this value is persisted to audit_logs.ip_address and sessions.ip_address, so reading the
// connection's own address instead would record loopback for every request forever.
func TestRequestInfo_RecordsForwardedClientAddress(t *testing.T) {
	t.Parallel()

	got := observedIP("127.0.0.1:54321", map[string]string{"X-Forwarded-For": "203.0.113.9"})

	assert.Equal(t, "203.0.113.9", got, "the client address must survive the proxy hop")
}

func TestRequestInfo_FallsBackToThePeerWithoutAProxy(t *testing.T) {
	t.Parallel()

	got := observedIP("198.51.100.7:44444", nil)

	assert.Equal(t, "198.51.100.7", got, "the port is stripped")
}

// TestRequestInfo_TakesTheFirstForwardedEntry covers a chain arriving from an upstream
// proxy in front of Caddy. Caddy itself sets a single value, so this is defensive.
func TestRequestInfo_TakesTheFirstForwardedEntry(t *testing.T) {
	t.Parallel()

	got := observedIP("127.0.0.1:54321", map[string]string{
		"X-Forwarded-For": "203.0.113.9, 10.0.0.4",
	})

	assert.Equal(t, "203.0.113.9", got)
}

func TestRequestInfo_IgnoresAnEmptyForwardedHeader(t *testing.T) {
	t.Parallel()

	got := observedIP("198.51.100.7:44444", map[string]string{"X-Forwarded-For": "   "})

	assert.Equal(t, "198.51.100.7", got)
}

// TestRequestInfo_IgnoresHeadersCaddyDoesNotManage pins that these are never read. Caddy
// deletes them, but nothing here should depend on that: reading them would let a client
// author the audit trail on any path that bypasses the proxy.
func TestRequestInfo_IgnoresHeadersCaddyDoesNotManage(t *testing.T) {
	t.Parallel()

	got := observedIP("127.0.0.1:54321", map[string]string{
		"X-Real-IP":      "8.8.8.8",
		"True-Client-IP": "8.8.4.4",
		"Forwarded":      "for=1.2.3.4",
	})

	assert.Equal(t, "127.0.0.1", got)
}

func TestRequestInfo_RecordsUserAgent(t *testing.T) {
	t.Parallel()
	var recorded string
	handler := mw.RequestInfo()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		recorded = requestmeta.UserAgentFrom(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("User-Agent", "dinchy-test/1.0")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "dinchy-test/1.0", recorded)
}
