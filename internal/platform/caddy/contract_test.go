package caddy_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
)

// These tests drive a real Caddy process. They exist because the defects this package is
// most likely to ship are semantic rather than structural: a field can be present, of the
// right type, and still fail to do anything. The bug that motivated them was of that kind
// — a certificate that loaded but was never served — and it is not expressible as a schema
// violation. Only a live handshake and a live request find it.
//
// They also catch drift: every Caddy release is an opportunity for a field name or module
// identifier in config.go to become wrong, which unit tests against our own structs
// cannot detect.
//
// Certificates come from Caddy's own local CA, exactly as they do in development, so these
// tests also cover the dev TLS setup end to end. Nothing is minted by the test.

// caddyBinary locates a runnable Caddy, skipping the test when there is none so a machine
// without Caddy still passes the suite. DINCHY_TEST_CADDY_BINARY is a test-only override
// for a CI image that installs Caddy somewhere else; it is not application configuration.
func caddyBinary(t *testing.T) string {
	t.Helper()
	candidates := []string{os.Getenv("DINCHY_TEST_CADDY_BINARY"), "tmp/caddy", "../../../tmp/caddy", "caddy"}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}
	t.Skip("no runnable Caddy binary found; run `task caddy:build`")
	return ""
}

// freePort reserves a port by binding and releasing it. Caddy needs concrete ports in its
// configuration, and the test must not collide with a developer's running `caddy:dev`.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

// localCARoots returns a pool trusting the root of Caddy's own local CA, which is what
// signs every certificate in development. Caddy writes it when the TLS app first
// provisions the internal issuer, so the file appears shortly after the configuration
// loads rather than at startup.
func localCARoots(t *testing.T, dataDir string) *x509.CertPool {
	t.Helper()
	rootPath := filepath.Join(dataDir, "caddy", "pki", "authorities", "local", "root.crt")

	var pem []byte
	require.Eventually(t, func() bool {
		contents, err := os.ReadFile(rootPath) //nolint:gosec // path is test-controlled
		if err != nil {
			return false
		}
		pem = contents
		return true
	}, 30*time.Second, 200*time.Millisecond, "caddy never provisioned its local CA at %s", rootPath)

	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(pem))
	return roots
}

// waitForCertificate blocks until Caddy has issued host's certificate. Issuance is
// asynchronous: POST /load returns before it finishes, so a request sent straight after a
// reconcile races it and fails the handshake with "tls: internal error".
func waitForCertificate(t *testing.T, dataDir, host string) {
	t.Helper()
	path := filepath.Join(dataDir, "caddy", "certificates", "local", host, host+".crt")
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 30*time.Second, 100*time.Millisecond,
		"caddy never issued a local certificate for %s; the usual cause is a host served by a route "+
			"but named by no automation policy, which falls through to the default ACME issuer", host)
}

// edgeServerName is the key of the edge's HTTP server in the test's base configuration. It mirrors
// deploy/caddy/base.dev.json, where the same name is the contract between the edge and its tenants.
const edgeServerName = "edge"

// liveCaddy is a running Caddy plus everything needed to talk to it and through it.
type liveCaddy struct {
	cfg     config.CaddyConfig
	admin   caddy.AdminClient
	dataDir string
	port    int
}

// startEdge launches an isolated Caddy carrying the edge's base configuration and returns once its
// admin API answers.
//
// The base configuration is not a formality: nothing pushes a whole document any more, and Caddy
// refuses to traverse into a path whose parents are absent. The server and its route array, and the
// automation policy array, all have to exist before a tenant can address anything inside them —
// which is exactly the contract deploy/caddy/base.dev.json satisfies in a real deployment.
func startEdge(t *testing.T, panelHost string) liveCaddy {
	t.Helper()
	binary := caddyBinary(t)
	dir := t.TempDir()

	adminPort, httpsPort := freePort(t), freePort(t)
	adminEndpoint := "127.0.0.1:" + strconv.Itoa(adminPort)

	// Caddy defaults its admin API to :2019, which would collide with a developer's running
	// instance, so the base configuration names the chosen port. Redirects are disabled for the
	// reason a development edge disables them: Caddy builds the redirect vhost on port 80 for
	// every HTTPS site regardless of the site's own port, and an unprivileged test process
	// cannot bind 80.
	bootstrap := filepath.Join(dir, "base.json")
	base := fmt.Sprintf(`{
	  "admin": {"listen": %q, "origins": [%q]},
	  "apps": {
	    "http": {"https_port": %d, "servers": {
	      %q: {"listen": [":%d"], "routes": [], "automatic_https": {"disable_redirects": true}}
	    }},
	    "tls": {"automation": {"policies": []}}
	  }
	}`, adminEndpoint, adminEndpoint, httpsPort, edgeServerName, httpsPort)
	require.NoError(t, os.WriteFile(bootstrap, []byte(base), 0o600))

	cmd := exec.Command(binary, "run", "--config", bootstrap) //nolint:gosec // path is test-controlled
	// Point Caddy's storage at the temp dir. Without this the test writes into the
	// developer's real certificate and ACME-account storage. --resume is deliberately
	// absent for the same reason: it would restore their dev configuration.
	dataDir := filepath.Join(dir, "data")
	cmd.Env = append(os.Environ(),
		"XDG_DATA_HOME="+dataDir,
		"XDG_CONFIG_HOME="+filepath.Join(dir, "config"),
	)
	var output strings.Builder
	cmd.Stdout, cmd.Stderr = &output, &output
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("caddy output:\n%s", output.String())
		}
	})

	cfg := config.DefaultCaddy()
	cfg.AdminEndpoint = adminEndpoint
	cfg.AdminTimeout = 15 * time.Second
	cfg.EdgeServerName = edgeServerName
	cfg.PanelHost = panelHost
	cfg.TLSIssuer = config.TLSIssuerInternal
	cfg.HSTSMaxAge = 0

	admin := caddy.NewAdminClient(cfg)
	ctx := context.Background()
	require.Eventually(t, func() bool { return admin.Ping(ctx) == nil }, 30*time.Second, 200*time.Millisecond,
		"caddy admin API never came up")

	return liveCaddy{cfg: cfg, admin: admin, dataDir: dataDir, port: httpsPort}
}

// client dials every host at the loopback port Caddy listens on, so the test does not
// depend on how the machine resolves the hostnames being served. It is built per request
// because Caddy's local CA root only exists once a configuration has been loaded.
func (l liveCaddy) client(t *testing.T) *http.Client {
	t.Helper()
	roots, port := localCARoots(t, l.dataDir), l.port
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			// Keep-alives off: a reused connection would keep answering from a route that
			// has since been removed, masking the change under test.
			DisableKeepAlives: true,
			TLSClientConfig:   &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:"+strconv.Itoa(port))
			},
		},
	}
}

// echoUpstream serves a description of the request it received, so the test can assert on
// the headers Caddy forwarded.
func echoUpstream(t *testing.T, name string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"upstream":          name,
			"host":              r.Host,
			"x-forwarded-proto": r.Header.Get("X-Forwarded-Proto"),
			"x-forwarded-for":   r.Header.Get("X-Forwarded-For"),
			"x-forwarded-host":  r.Header.Get("X-Forwarded-Host"),
			"x-real-ip":         r.Header.Get("X-Real-IP"),
			"true-client-ip":    r.Header.Get("True-Client-IP"),
			"forwarded":         r.Header.Get("Forwarded"),
		})
	}))
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "http://")
}

// getRaw performs a request through Caddy and returns the status and the body as text.
func (l liveCaddy) getRaw(t *testing.T, host, path string) (status int, body string) {
	t.Helper()
	waitForCertificate(t, l.dataDir, host)
	url := fmt.Sprintf("https://%s:%d%s", host, l.port, path)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)

	resp, err := l.client(t).Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(raw)
}

// get performs a request through Caddy and returns the status and decoded body.
func (l liveCaddy) get(t *testing.T, host string) (status int, body map[string]string) {
	t.Helper()
	status, raw := l.getRaw(t, host, "/")
	body = map[string]string{}
	_ = json.Unmarshal([]byte(raw), &body)
	return status, body
}

// apply pushes a route set as this tenant's two objects, the way the reconciler does.
func (l liveCaddy) apply(ctx context.Context, routes ...caddy.Route) error {
	contribution, err := caddy.BuildContribution(l.cfg, routes)
	if err != nil {
		return err
	}
	if err := l.admin.ApplyTLSPolicy(ctx, contribution.Policy); err != nil {
		return err
	}
	return l.admin.ApplyRoute(ctx, l.cfg.EdgeServerName, contribution.Route)
}

// tenantOn returns a second deployment's configuration against the same running edge, differing
// only in the two values that make it a distinct tenant.
func (l liveCaddy) tenantOn(tenant, panelHost string) config.CaddyConfig {
	cfg := l.cfg
	cfg.Tenant = tenant
	cfg.PanelHost = panelHost
	return cfg
}

// TestContract_TwoTenantsShareOneEdgeWithoutClobberingEachOther is the test this whole change
// exists for. Dinchy used to POST a whole document to /load, which replaced every other tenant's
// routes; addressing one object each is what makes a shared edge possible.
func TestContract_TwoTenantsShareOneEdgeWithoutClobberingEachOther(t *testing.T) {
	live := startEdge(t, "alpha.test")
	ctx := context.Background()

	alpha := live
	alpha.cfg = live.tenantOn("alpha", "alpha.test")
	beta := live
	beta.cfg = live.tenantOn("beta", "beta.test")

	require.NoError(t, alpha.apply(ctx, caddy.Route{
		Owner: caddy.PanelOwner, Host: "alpha.test", Upstream: echoUpstream(t, "alpha"),
	}))
	_, body := alpha.get(t, "alpha.test")
	require.Equal(t, "alpha", body["upstream"])

	require.NoError(t, beta.apply(ctx, caddy.Route{
		Owner: caddy.PanelOwner, Host: "beta.test", Upstream: echoUpstream(t, "beta"),
	}))

	// Both serve their own upstream, and alpha survived a write it knew nothing about.
	_, betaBody := beta.get(t, "beta.test")
	assert.Equal(t, "beta", betaBody["upstream"])
	_, alphaBody := alpha.get(t, "alpha.test")
	assert.Equal(t, "alpha", alphaBody["upstream"], "beta's push must not disturb alpha")

	running := live.fetchConfig(t)
	assert.ElementsMatch(t, []string{"dinchy.alpha.routes", "dinchy.beta.routes"},
		live.tenantObjectIDs(t, running))
	assert.ElementsMatch(t, []string{"dinchy.alpha.tls", "dinchy.beta.tls"},
		live.policyObjectIDs(t, running))
}

// TestContract_RepushingATenantDoesNotDuplicateItsObjects covers a restart, and the pruning that
// makes a Remove operation unnecessary. Appending on every boot would grow the edge's autosaved
// configuration without bound, and a route a source stopped reporting has to actually stop serving.
func TestContract_RepushingATenantDoesNotDuplicateItsObjects(t *testing.T) {
	live := startEdge(t, "alpha.test")
	ctx := context.Background()
	live.cfg = live.tenantOn("alpha", "alpha.test")

	panel := caddy.Route{Owner: caddy.PanelOwner, Host: "alpha.test", Upstream: echoUpstream(t, "alpha")}
	extra := caddy.Route{Owner: "deployments", Host: "extra.test", Upstream: echoUpstream(t, "extra")}

	require.NoError(t, live.apply(ctx, panel))
	require.NoError(t, live.apply(ctx, panel), "a second push models a process restart")

	running := live.fetchConfig(t)
	assert.Equal(t, []string{"dinchy.alpha.routes"}, live.tenantObjectIDs(t, running))
	assert.Equal(t, []string{"dinchy.alpha.tls"}, live.policyObjectIDs(t, running))

	// Adding a route converges without touching the object count.
	require.NoError(t, live.apply(ctx, panel, extra))
	_, body := live.get(t, "extra.test")
	assert.Equal(t, "extra", body["upstream"])
	assert.Equal(t, []string{"dinchy.alpha.routes"}, live.tenantObjectIDs(t, live.fetchConfig(t)))

	// Dropping it removes it, which is the pruning a Remove operation would otherwise do. The
	// running configuration is the assertion rather than a request: an unserved host fails the
	// handshake instead of answering with a status, so there is nothing to read from the wire.
	require.NoError(t, live.apply(ctx, panel))
	assert.Equal(t, []string{"alpha.test"}, live.routeHosts(t, live.fetchConfig(t)),
		"a route no source reports must be gone")
	_, panelBody := live.get(t, "alpha.test")
	assert.Equal(t, "alpha", panelBody["upstream"], "pruning must not take the panel with it")
}

// TestContract_ScopedWriteRejectionLeavesTheOtherTenantServing pins that Caddy provisions the whole
// resulting configuration and rolls back on failure. That is what lets this package delegate
// structural validation — and it is what keeps one tenant's bad push from being everyone's outage.
func TestContract_ScopedWriteRejectionLeavesTheOtherTenantServing(t *testing.T) {
	live := startEdge(t, "alpha.test")
	ctx := context.Background()

	alpha := live
	alpha.cfg = live.tenantOn("alpha", "alpha.test")
	require.NoError(t, alpha.apply(ctx, caddy.Route{
		Owner: caddy.PanelOwner, Host: "alpha.test", Upstream: echoUpstream(t, "alpha"),
	}))

	// An unknown handler module is refused at provision time, unlike a bad dial address.
	badRoute := caddy.ServerRoute{
		ID:       caddy.RouteObjectID("beta"),
		Match:    []caddy.RouteMatch{{Host: []string{"beta.test"}}},
		Handle:   []caddy.RouteHandler{{Handler: "no_such_handler"}},
		Terminal: true,
	}
	err := live.admin.ApplyRoute(ctx, live.cfg.EdgeServerName, badRoute)
	assertCode(t, err, i18n.CodePlatformRoutingConfigRejected, http.StatusInternalServerError)

	_, body := alpha.get(t, "alpha.test")
	assert.Equal(t, "alpha", body["upstream"], "a rejected write must roll back, not take the edge down")
}

// TestContract_AbsentObjectIDIsDistinguishableFromAFailure pins the one status that means "append
// instead of replace". The admin client branches on it, and a Caddy upgrade that changed it would
// otherwise turn every restart into a duplicated route.
func TestContract_AbsentObjectIDIsDistinguishableFromAFailure(t *testing.T) {
	live := startEdge(t, "alpha.test")

	status, body := live.adminGet(t, "/id/dinchy.nonexistent.routes")

	assert.Equal(t, http.StatusNotFound, status, "an unknown @id is a 404, and nothing else is")
	assert.Contains(t, body, "unknown object ID")
}

// TestContract_MissingParentPathIsRejected proves the base-configuration contract fails loudly. The
// fix is in the edge's configuration, not in the route, so it carries its own message.
func TestContract_MissingParentPathIsRejected(t *testing.T) {
	live := startEdge(t, "alpha.test")
	ctx := context.Background()
	live.cfg.EdgeServerName = "no-such-server"

	err := live.apply(ctx, caddy.Route{
		Owner: caddy.PanelOwner, Host: "alpha.test", Upstream: echoUpstream(t, "alpha"),
	})

	assertCode(t, err, i18n.CodePlatformRoutingBaseConfigInvalid, http.StatusInternalServerError)
}

func TestContract_RealCaddyServesTheGeneratedConfiguration(t *testing.T) {
	live := startEdge(t, "panel.test")
	ctx := context.Background()
	panelUpstream := echoUpstream(t, "panel")

	reconciler, err := caddy.NewReconciler(live.cfg, live.admin)
	require.NoError(t, err)
	reconciler.Register(caddy.NewStaticSource(caddy.PanelOwner, caddy.Route{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: panelUpstream,
	}))

	// 1. A real Caddy accepts the document. This is what catches field-name and module-ID
	//    drift when Caddy is upgraded.
	result, err := reconciler.ReconcileAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, result.RouteCount)

	// 2. The TLS handshake returns the loaded certificate. Without a connection policy
	//    selecting it, Caddy has nothing to offer for this SNI and aborts the handshake —
	//    the bug this assertion exists for.
	status, body := live.get(t, live.cfg.PanelHost)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "panel", body["upstream"])

	// 3. The forwarded headers arrive as configured. This is now the only place these
	//    headers are normalized, so nothing else would notice if it broke.
	assert.Equal(t, "https", body["x-forwarded-proto"], "the app derives request security from this")
	assert.NotEmpty(t, body["x-forwarded-for"], "the app records this as the client IP in audit rows")
	// Caddy does not strip these by default; they are absent only because the generated
	// configuration deletes them.
	assert.Empty(t, body["x-real-ip"])
	assert.Empty(t, body["true-client-ip"])
	assert.Empty(t, body["forwarded"])

	// Host is forwarded verbatim, port included. CORS compares the browser's Origin
	// against scheme://r.Host, so dropping or rewriting the port would 403 every
	// cross-origin request on a non-default port.
	assert.Equal(t, fmt.Sprintf("%s:%d", live.cfg.PanelHost, live.port), body["host"])
	// X-Forwarded-Host carries only the hostname: Caddy's {http.request.host} placeholder
	// excludes the port, unlike the Host header. Nothing depends on it today, so this
	// records the difference rather than asserting they match.
	assert.Equal(t, live.cfg.PanelHost, body["x-forwarded-host"])
}

func TestContract_ConfigurationRoundTripsThroughCaddy(t *testing.T) {
	live := startEdge(t, "panel.test")
	ctx := context.Background()

	require.NoError(t, live.apply(ctx, caddy.Route{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: echoUpstream(t, "panel"),
	}))

	running := live.fetchConfig(t)

	// The edge owns the admin block, and a scoped write must not touch it. Losing it would
	// tear down the very endpoint used to push, leaving no way back without editing files.
	admin, ok := running["admin"].(map[string]any)
	require.True(t, ok, "the running configuration must keep the edge's admin block")
	assert.Equal(t, live.cfg.AdminEndpoint, admin["listen"])

	// Caddy kept the route inside the edge's server, matching our host.
	assert.Contains(t, live.routeHosts(t, running), live.cfg.PanelHost)
}

// TestContract_ReconcileConvergesASecondRoute covers the path a drift repair takes: a
// source reports a new route and the next full reconcile serves it, with the panel
// untouched throughout.
func TestContract_ReconcileConvergesASecondRoute(t *testing.T) {
	live := startEdge(t, "panel.test")
	ctx := context.Background()

	reconciler, err := caddy.NewReconciler(live.cfg, live.admin)
	require.NoError(t, err)
	reconciler.Register(caddy.NewStaticSource(caddy.PanelOwner, caddy.Route{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: echoUpstream(t, "panel"),
	}))
	_, err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	reconciler.Register(caddy.NewStaticSource("deployments", caddy.Route{
		Owner: "deployments", Host: "whoami.apps.test", Upstream: echoUpstream(t, "deployment"),
	}))
	_, err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	status, body := live.get(t, "whoami.apps.test")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "deployment", body["upstream"])

	_, panelBody := live.get(t, live.cfg.PanelHost)
	assert.Equal(t, "panel", panelBody["upstream"], "converging a new route must not disturb the panel")
}

// TestContract_TheWebUIAndAPIShareOneHost is the check behind the frontend split. The edge forwards
// /api to Dinchy and everything else to whatever serves the built assets, which is what keeps the
// browser same-origin — and same-origin is what lets the session and CSRF cookies stay
// SameSite=Lax and keeps CORS uninvolved.
//
// Both are proxied. The edge is shared between applications and can read none of their files, so
// there is no disk path here to get wrong.
func TestContract_TheWebUIAndAPIShareOneHost(t *testing.T) {
	live := startEdge(t, "panel.test")
	ctx := context.Background()

	reconciler, err := caddy.NewReconciler(live.cfg, live.admin)
	require.NoError(t, err)
	reconciler.Register(caddy.NewStaticSource(caddy.PanelOwner,
		caddy.Route{
			Owner: caddy.PanelOwner, Host: live.cfg.PanelHost,
			PathPrefix: "/api", Upstream: echoUpstream(t, "api"),
		},
		caddy.Route{
			Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: echoUpstream(t, "web"),
		},
	))
	_, err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	// Anything outside /api reaches the frontend, including a client-side route with no file
	// behind it — the fallback is that upstream's job, not the edge's.
	status, body := live.getRaw(t, live.cfg.PanelHost, "/settings/organization")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"upstream":"web"`)

	// The API prefix reaches Dinchy, on the same hostname.
	status, body = live.getRaw(t, live.cfg.PanelHost, "/api/anything")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"upstream":"api"`, "the more specific prefix must win")
}

// TestContract_CaddyAcceptsAMalformedUpstreamAndFailsPerRequest records where Caddy stops
// being a validator. It provisions and rolls back a configuration it cannot load, which is
// what lets Dinchy delegate structural checks — but a reverse-proxy dial address is not
// among them. Caddy parses it lazily, per request, so a typo loads cleanly and every
// request through the route 502s with nothing reported at push time.
//
// Anything that composes a Route from user input therefore has to screen the upstream
// itself; there is no backstop here.
func TestContract_CaddyAcceptsAMalformedUpstreamAndFailsPerRequest(t *testing.T) {
	live := startEdge(t, "panel.test")
	ctx := context.Background()

	require.NoError(t, live.apply(ctx, caddy.Route{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: "not a host:8080",
	}), "Caddy does not check dial addresses at load time")

	status, _ := live.getRaw(t, live.cfg.PanelHost, "/")
	assert.Equal(t, http.StatusBadGateway, status, "the failure surfaces per request instead")
}

// adminGet performs a raw admin API request, for pinning Caddy's own error shapes.
func (l liveCaddy) adminGet(t *testing.T, path string) (status int, body string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://"+l.cfg.AdminEndpoint+path, http.NoBody)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(raw)
}

// fetchConfig reads the configuration Caddy is currently running.
func (l liveCaddy) fetchConfig(t *testing.T) map[string]any {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://"+l.cfg.AdminEndpoint+"/config/", http.NoBody)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var running map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&running))
	return running
}

// routeHosts extracts the hosts this tenant's entrypoints match, inside the edge's server.
func (l liveCaddy) routeHosts(t *testing.T, running map[string]any) []string {
	t.Helper()
	apps, ok := running["apps"].(map[string]any)
	require.True(t, ok)
	httpApp, ok := apps["http"].(map[string]any)
	require.True(t, ok)
	servers, ok := httpApp["servers"].(map[string]any)
	require.True(t, ok)
	server, ok := servers[l.cfg.EdgeServerName].(map[string]any)
	require.True(t, ok, "the edge's server must survive a tenant's write")
	routes, ok := server["routes"].([]any)
	require.True(t, ok)

	// Descend into this tenant's wrapper route: the hosts live on the nested entrypoints, and
	// the sibling entries belong to other tenants.
	var hosts []string
	for _, entry := range routes {
		route, ok := entry.(map[string]any)
		if !ok || route["@id"] != caddy.RouteObjectID(l.cfg.Tenant) {
			continue
		}
		for _, handler := range asSlice(route["handle"]) {
			handlerMap, ok := handler.(map[string]any)
			if !ok {
				continue
			}
			for _, nested := range asSlice(handlerMap["routes"]) {
				hosts = append(hosts, matchedHosts(nested)...)
			}
		}
	}
	return hosts
}

// tenantObjectIDs returns the "@id" of every route the edge is serving, across all tenants.
func (l liveCaddy) tenantObjectIDs(t *testing.T, running map[string]any) []string {
	t.Helper()
	apps, ok := running["apps"].(map[string]any)
	require.True(t, ok)
	httpApp, ok := apps["http"].(map[string]any)
	require.True(t, ok)
	servers, ok := httpApp["servers"].(map[string]any)
	require.True(t, ok)
	server, ok := servers[l.cfg.EdgeServerName].(map[string]any)
	require.True(t, ok)

	var ids []string
	for _, entry := range asSlice(server["routes"]) {
		if route, ok := entry.(map[string]any); ok {
			if id, ok := route["@id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// policyObjectIDs returns the "@id" of every certificate automation policy on the edge.
func (l liveCaddy) policyObjectIDs(t *testing.T, running map[string]any) []string {
	t.Helper()
	apps, ok := running["apps"].(map[string]any)
	require.True(t, ok)
	tlsApp, ok := apps["tls"].(map[string]any)
	require.True(t, ok)
	automation, ok := tlsApp["automation"].(map[string]any)
	require.True(t, ok)

	var ids []string
	for _, entry := range asSlice(automation["policies"]) {
		if policy, ok := entry.(map[string]any); ok {
			if id, ok := policy["@id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func asSlice(value any) []any {
	slice, _ := value.([]any)
	return slice
}

// matchedHosts returns the hosts one decoded route matches on.
func matchedHosts(entry any) []string {
	route, ok := entry.(map[string]any)
	if !ok {
		return nil
	}
	matches := asSlice(route["match"])
	if len(matches) == 0 {
		return nil
	}
	match, ok := matches[0].(map[string]any)
	if !ok {
		return nil
	}
	var hosts []string
	for _, matched := range asSlice(match["host"]) {
		if host, ok := matched.(string); ok {
			hosts = append(hosts, host)
		}
	}
	return hosts
}
