package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	transport "github.com/sidarth-23/dinchy/internal/transport"
)

var fixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	sharedService := features.Service{Clock: clock.Fixed(fixedTime), IDGenerator: id.NewGenerator(), CacheKeyer: cache.NewKeyer("test")}
	sessionSvc, err := session.NewService(sharedService.Named("session"), nil, config.DefaultSession(), config.DefaultCache())
	require.NoError(t, err)
	svc, err := auth.NewService(sharedService.Named("auth"), nil, sessionSvc, config.DefaultAuth(), config.NewLinks(""), nil)
	require.NoError(t, err)
	srv := transport.New(transport.Options{
		Addr:          ":0",
		SecureCookies: true,
		PublicScheme:  "https",
	}, svc, sessionSvc, nil, nil)
	return srv.Handler
}

func doRequest(t *testing.T, handler http.Handler, method, path string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, "http://example.test"+path, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Result()
}

// TestNew_DoesNotServeTheWebUI pins the split: Caddy delivers the assets, so a document
// request reaching Dinchy means routing is wrong and should say so rather than be quietly
// answered by a second file server.
func TestNew_DoesNotServeTheWebUI(t *testing.T) {
	t.Parallel()
	handler := newTestServer(t)

	for _, path := range []string{"/", "/index.html", "/assets/app.js", "/some/client/route"} {
		resp := doRequest(t, handler, http.MethodGet, path)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "path %q must not be served here", path)
	}
}

func TestNew_ServesTheAPIUnderItsPrefix(t *testing.T) {
	t.Parallel()
	handler := newTestServer(t)

	resp := doRequest(t, handler, http.MethodGet, transport.APIPathPrefix+"/auth/sso/providers")
	defer func() { _ = resp.Body.Close() }()

	assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "the API prefix must reach the Huma router")
}
