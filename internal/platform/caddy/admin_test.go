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

func TestAdminClient_LoadConfigPostsJSONToLoad(t *testing.T) {
	fake := newFakeAdmin(t)
	client := caddy.NewAdminClient(fake.clientConfig())

	built, err := caddy.BuildConfig(productionConfig(), []caddy.Route{panelRoute("panel.example.com", "127.0.0.1:8080")})
	require.NoError(t, err)
	require.NoError(t, client.LoadConfig(context.Background(), built))

	calls := fake.recorded()
	require.Len(t, calls, 1)
	assert.Equal(t, http.MethodPost, calls[0].Method)
	assert.Equal(t, "/load", calls[0].Path)

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(calls[0].Body), &sent))
	assert.Contains(t, sent, "admin", "the loaded document must keep the admin endpoint alive")
}

func TestAdminClient_PutRoutePatchesExistingRouteByID(t *testing.T) {
	fake := newFakeAdmin(t)
	client := caddy.NewAdminClient(fake.clientConfig())

	route := caddy.ServerRoute{ID: "dinchy-deployments-app.example.com-root"}
	require.NoError(t, client.PutRoute(context.Background(), route))

	calls := fake.recorded()
	require.Len(t, calls, 1, "an existing route is patched in place, not appended")
	assert.Equal(t, http.MethodPatch, calls[0].Method)
	assert.Equal(t, "/id/dinchy-deployments-app.example.com-root", calls[0].Path)
}

func TestAdminClient_PutRouteAppendsWhenTheIDIsUnknown(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.respondWith(func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPatch {
			return false
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"unknown object ID '/id/dinchy-new'"}`))
		return true
	})
	client := caddy.NewAdminClient(fake.clientConfig())

	require.NoError(t, client.PutRoute(context.Background(), caddy.ServerRoute{ID: "dinchy-new"}))

	calls := fake.recorded()
	require.Len(t, calls, 2)
	assert.Equal(t, http.MethodPatch, calls[0].Method)
	assert.Equal(t, http.MethodPost, calls[1].Method)
	assert.Equal(t, "/config/apps/http/servers/"+caddy.ServerName+"/routes", calls[1].Path)
}

func TestAdminClient_DeleteRouteTreatsMissingRouteAsDone(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.respondWith(func(w http.ResponseWriter, _ *http.Request) bool {
		w.WriteHeader(http.StatusNotFound)
		return true
	})
	client := caddy.NewAdminClient(fake.clientConfig())

	// The desired end state is reached either way, so an absent route is not an error.
	require.NoError(t, client.DeleteRoute(context.Background(), "dinchy-gone"))
}

func TestAdminClient_RejectedConfigurationReportsConfigRejected(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.respondWith(func(w http.ResponseWriter, _ *http.Request) bool {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"loading http app module: invalid listener"}`))
		return true
	})
	client := caddy.NewAdminClient(fake.clientConfig())

	err := client.LoadConfig(context.Background(), caddy.Config{})

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

	// An operator needs "the proxy is not reachable", not "the reload failed".
	assertCode(t, err, i18n.CodePlatformRoutingUnavailable, http.StatusInternalServerError)
}
