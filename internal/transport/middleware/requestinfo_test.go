package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/foundation/requestmeta"
	mw "github.com/sidarth-23/dinchy/internal/transport/middleware"
)

// loopbackTrust is the default trust set: what a host-native deployment behind the edge has.
func loopbackTrust(t *testing.T) []netip.Prefix {
	t.Helper()
	return prefixes(t, "127.0.0.1/32", "::1/128")
}

func prefixes(t *testing.T, values ...string) []netip.Prefix {
	t.Helper()
	parsed := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		require.NoError(t, err)
		parsed = append(parsed, prefix)
	}
	return parsed
}

// observedIP runs a request through RequestInfo and reports the address it recorded.
func observedIP(trusted []netip.Prefix, remoteAddr string, headers map[string]string) string {
	var recorded string
	handler := mw.RequestInfo(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
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

// TestRequestInfo_RecordsForwardedClientAddressFromATrustedPeer is the regression guard for
// durable data: this value is persisted to audit_logs.ip_address and sessions.ip_address, so
// reading the connection's own address instead would record the edge for every request forever.
func TestRequestInfo_RecordsForwardedClientAddressFromATrustedPeer(t *testing.T) {
	t.Parallel()

	got := observedIP(loopbackTrust(t), "127.0.0.1:54321", map[string]string{"X-Forwarded-For": "203.0.113.9"})

	assert.Equal(t, "203.0.113.9", got, "the client address must survive the proxy hop")
}

// TestRequestInfo_IgnoresForwardedHeaderFromAnUntrustedPeer is the security assertion this trust
// set exists for. The listener may face the edge network, so anything else on that network can
// reach it directly; if its header were honored it would be choosing what the audit log records
// about it.
func TestRequestInfo_IgnoresForwardedHeaderFromAnUntrustedPeer(t *testing.T) {
	t.Parallel()

	got := observedIP(loopbackTrust(t), "10.89.100.5:33333", map[string]string{"X-Forwarded-For": "1.2.3.4"})

	assert.Equal(t, "10.89.100.5", got, "an untrusted peer must not be able to author its own address")
}

func TestRequestInfo_HonoursForwardedHeaderFromATrustedNetwork(t *testing.T) {
	t.Parallel()

	got := observedIP(prefixes(t, "10.89.100.0/24"), "10.89.100.5:33333",
		map[string]string{"X-Forwarded-For": "203.0.113.9"})

	assert.Equal(t, "203.0.113.9", got)
}

// TestRequestInfo_TrustsAnIPv4MappedLoopbackPeer covers the trap that would silently disable the
// whole trust set: on a dual-stack listener a loopback peer arrives as ::ffff:127.0.0.1, which no
// 127.0.0.1/32 prefix matches unless the address is unmapped first.
func TestRequestInfo_TrustsAnIPv4MappedLoopbackPeer(t *testing.T) {
	t.Parallel()

	got := observedIP(loopbackTrust(t), "[::ffff:127.0.0.1]:54321",
		map[string]string{"X-Forwarded-For": "203.0.113.9"})

	assert.Equal(t, "203.0.113.9", got)
}

func TestRequestInfo_TreatsAnUnparseablePeerAsUntrusted(t *testing.T) {
	t.Parallel()

	got := observedIP(loopbackTrust(t), "@", map[string]string{"X-Forwarded-For": "1.2.3.4"})

	assert.Equal(t, "@", got, "a peer that cannot be classified is not trusted")
}

func TestRequestInfo_AnEmptyTrustSetAlwaysRecordsThePeer(t *testing.T) {
	t.Parallel()

	got := observedIP(nil, "127.0.0.1:54321", map[string]string{"X-Forwarded-For": "1.2.3.4"})

	assert.Equal(t, "127.0.0.1", got)
}

func TestRequestInfo_FallsBackToThePeerWithoutAProxy(t *testing.T) {
	t.Parallel()

	got := observedIP(prefixes(t, "198.51.100.0/24"), "198.51.100.7:44444", nil)

	assert.Equal(t, "198.51.100.7", got, "the port is stripped")
}

// TestRequestInfo_TakesTheFirstForwardedEntry covers a chain arriving from an upstream proxy in
// front of the edge. The edge itself sets a single value, so this is defensive.
func TestRequestInfo_TakesTheFirstForwardedEntry(t *testing.T) {
	t.Parallel()

	got := observedIP(loopbackTrust(t), "127.0.0.1:54321", map[string]string{
		"X-Forwarded-For": "203.0.113.9, 10.0.0.4",
	})

	assert.Equal(t, "203.0.113.9", got)
}

func TestRequestInfo_IgnoresAnEmptyForwardedHeader(t *testing.T) {
	t.Parallel()

	got := observedIP(loopbackTrust(t), "127.0.0.1:44444", map[string]string{"X-Forwarded-For": "   "})

	assert.Equal(t, "127.0.0.1", got)
}

// TestRequestInfo_IgnoresHeadersCaddyDoesNotManage pins that these are never read. Caddy deletes
// them, but nothing here should depend on that: reading them would let a client author the audit
// trail on any path that bypasses the proxy.
func TestRequestInfo_IgnoresHeadersCaddyDoesNotManage(t *testing.T) {
	t.Parallel()

	got := observedIP(loopbackTrust(t), "127.0.0.1:54321", map[string]string{
		"X-Real-IP":      "8.8.8.8",
		"True-Client-IP": "8.8.4.4",
		"Forwarded":      "for=1.2.3.4",
	})

	assert.Equal(t, "127.0.0.1", got)
}

func TestRequestInfo_RecordsUserAgent(t *testing.T) {
	t.Parallel()
	var recorded string
	handler := mw.RequestInfo(loopbackTrust(t))(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		recorded = requestmeta.UserAgentFrom(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("User-Agent", "dinchy-test/1.0")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "dinchy-test/1.0", recorded)
}
