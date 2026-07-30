package caddy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
)

// recordedCall is one admin API request the fake Caddy received.
type recordedCall struct {
	Method string
	Path   string
	Body   string
}

// fakeAdmin is a stand-in for Caddy's admin API.
type fakeAdmin struct {
	mu       sync.Mutex
	calls    []recordedCall
	handler  func(w http.ResponseWriter, r *http.Request) bool
	server   *httptest.Server
	endpoint string
}

func newFakeAdmin(t *testing.T) *fakeAdmin {
	t.Helper()
	fake := &fakeAdmin{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.calls = append(fake.calls, recordedCall{Method: r.Method, Path: r.URL.Path, Body: string(body)})
		handler := fake.handler
		fake.mu.Unlock()
		if handler != nil && handler(w, r) {
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(fake.server.Close)
	fake.endpoint = strings.TrimPrefix(fake.server.URL, "http://")
	return fake
}

func (f *fakeAdmin) recorded() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedCall(nil), f.calls...)
}

func (f *fakeAdmin) respondWith(handler func(w http.ResponseWriter, r *http.Request) bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = handler
}

func (f *fakeAdmin) clientConfig() config.CaddyConfig {
	cfg := productionConfig()
	cfg.AdminEndpoint = f.endpoint
	cfg.AdminTimeout = 5 * time.Second
	return cfg
}

// presentIDs makes the fake answer the existence probe for a set of "@id" values, so a test can
// model both a first push and a re-push.
func (f *fakeAdmin) presentIDs(ids ...string) {
	known := make(map[string]bool, len(ids))
	for _, id := range ids {
		known[id] = true
	}
	f.respondWith(func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/id/") {
			return false
		}
		if known[strings.TrimPrefix(r.URL.Path, "/id/")] {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"@id":"present"}`))
			return true
		}
		// Caddy's answer for an "@id" that is not in the running configuration.
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"unknown object ID"}`))
		return true
	})
}

// writes returns the calls that changed configuration, dropping the existence probes.
func (f *fakeAdmin) writes() []recordedCall {
	var written []recordedCall
	for _, call := range f.recorded() {
		if call.Method != http.MethodGet {
			written = append(written, call)
		}
	}
	return written
}

func testRoute(tenant string) caddy.ServerRoute {
	return caddy.ServerRoute{ID: caddy.RouteObjectID(tenant), Terminal: true}
}

func testPolicy(tenant string) caddy.AutomationPolicy {
	return caddy.AutomationPolicy{ID: caddy.TLSPolicyObjectID(tenant), Subjects: []string{"panel.example.com"}}
}

// TestAdminClient_ApplyRouteAppendsWhenAbsent covers a first push: with no object under the "@id"
// there is nothing to replace, so it is appended to the server's route array.
func TestAdminClient_ApplyRouteAppendsWhenAbsent(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.presentIDs()
	client := caddy.NewAdminClient(fake.clientConfig())

	require.NoError(t, client.ApplyRoute(context.Background(), "edge", testRoute("dinchy")))

	writes := fake.writes()
	require.Len(t, writes, 1)
	assert.Equal(t, http.MethodPost, writes[0].Method)
	assert.Equal(t, "/config/apps/http/servers/edge/routes", writes[0].Path)

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(writes[0].Body), &sent))
	assert.Equal(t, "dinchy.dinchy.routes", sent["@id"], "the object must carry its own address")
}

// TestAdminClient_ApplyRoutePatchesWhenPresent covers a restart. Appending again would add a second
// copy of this deployment's route on every boot, forever, in a configuration the edge autosaves.
func TestAdminClient_ApplyRoutePatchesWhenPresent(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.presentIDs(caddy.RouteObjectID("dinchy"))
	client := caddy.NewAdminClient(fake.clientConfig())

	require.NoError(t, client.ApplyRoute(context.Background(), "edge", testRoute("dinchy")))

	writes := fake.writes()
	require.Len(t, writes, 1)
	assert.Equal(t, http.MethodPatch, writes[0].Method)
	assert.Equal(t, "/id/dinchy.dinchy.routes", writes[0].Path)
}

func TestAdminClient_ApplyTLSPolicyAddressesThePolicyArray(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.presentIDs()
	client := caddy.NewAdminClient(fake.clientConfig())

	require.NoError(t, client.ApplyTLSPolicy(context.Background(), testPolicy("dinchy")))

	writes := fake.writes()
	require.Len(t, writes, 1)
	assert.Equal(t, http.MethodPost, writes[0].Method)
	assert.Equal(t, "/config/apps/tls/automation/policies", writes[0].Path)
}

// TestAdminClient_MissingParentPathReportsBaseConfigInvalid separates the operator's configuration
// of the edge from the object being written. Caddy answers both with 500, so only the body tells
// them apart, and the fix for this one is in the edge's base configuration.
func TestAdminClient_MissingParentPathReportsBaseConfigInvalid(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.respondWith(func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"unknown object ID"}`))
			return true
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"invalid traversal path at: config/apps/http/servers/missing"}`))
		return true
	})
	client := caddy.NewAdminClient(fake.clientConfig())

	err := client.ApplyRoute(context.Background(), "missing", testRoute("dinchy"))

	assertCode(t, err, i18n.CodePlatformRoutingBaseConfigInvalid, http.StatusInternalServerError)
}

func TestAdminClient_RejectedConfigurationReportsConfigRejected(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.respondWith(func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"unknown object ID"}`))
			return true
		}
		// A scoped write answers 500, not 400, when Caddy refuses to provision the result.
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"loading new config: loading http app module: invalid listener"}`))
		return true
	})
	client := caddy.NewAdminClient(fake.clientConfig())

	err := client.ApplyRoute(context.Background(), "edge", testRoute("dinchy"))

	assertCode(t, err, i18n.CodePlatformRoutingConfigRejected, http.StatusInternalServerError)
	assert.Contains(t, err.(interface{ Unwrap() error }).Unwrap().Error(), "invalid listener",
		"Caddy's own explanation is preserved as the diagnostic cause")
}

func TestAdminClient_UnreachableProxyReportsUnavailable(t *testing.T) {
	cfg := productionConfig()
	// A port nothing listens on: the transport fails before any HTTP status exists.
	cfg.AdminEndpoint = "127.0.0.1:1"
	cfg.AdminTimeout = time.Second
	client := caddy.NewAdminClient(cfg)

	err := client.Ping(context.Background())

	// An operator needs "the proxy is not reachable", not "the change failed".
	assertCode(t, err, i18n.CodePlatformRoutingUnavailable, http.StatusInternalServerError)
}
