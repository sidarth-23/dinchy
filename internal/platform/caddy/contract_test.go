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
	t.Skip("no runnable Caddy binary found; run `mise run caddy:build`")
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
	}, 30*time.Second, 100*time.Millisecond, "caddy never issued a certificate for %s", host)
}

// liveCaddy is a running Caddy plus everything needed to talk to it and through it.
type liveCaddy struct {
	cfg     config.CaddyConfig
	admin   caddy.AdminClient
	dataDir string
	port    int
}

// startCaddy launches an isolated Caddy on the given panel host and returns once its admin
// API answers.
func startCaddy(t *testing.T, panelHost string) liveCaddy {
	t.Helper()
	binary := caddyBinary(t)
	dir := t.TempDir()

	adminPort, httpsPort := freePort(t), freePort(t)
	adminEndpoint := "127.0.0.1:" + strconv.Itoa(adminPort)

	// Caddy defaults its admin API to :2019, which would collide with a developer's
	// running instance, so give it an initial configuration naming the chosen port. The
	// pushed configuration repeats the same endpoint, so loading it does not move it.
	bootstrap := filepath.Join(dir, "bootstrap.json")
	require.NoError(t, os.WriteFile(bootstrap,
		[]byte(`{"admin":{"listen":"`+adminEndpoint+`"}}`), 0o600))

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
	cfg.HTTPSPort = uint16(httpsPort) //nolint:gosec // freePort returns a valid port
	cfg.PanelHost = panelHost
	cfg.TLSIssuer = config.TLSIssuerInternal
	cfg.HSTSMaxAge = 0
	cfg.StoragePath = ""

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

func TestContract_RealCaddyServesTheGeneratedConfiguration(t *testing.T) {
	live := startCaddy(t, "panel.test")
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
	live := startCaddy(t, "panel.test")
	ctx := context.Background()

	built, err := caddy.BuildConfig(live.cfg, []caddy.Route{{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: echoUpstream(t, "panel"),
	}})
	require.NoError(t, err)
	require.NoError(t, live.admin.LoadConfig(ctx, built))

	running := live.fetchConfig(t)

	// The admin endpoint must survive the load. A document omitting it would tear down
	// the very endpoint used to push, leaving no way back without editing files by hand.
	admin, ok := running["admin"].(map[string]any)
	require.True(t, ok, "the running configuration must keep an admin block")
	assert.Equal(t, live.cfg.AdminEndpoint, admin["listen"])

	// Caddy kept the route under our server name and matching our host.
	assert.Contains(t, live.routeHosts(t, running), live.cfg.PanelHost)
}

// TestContract_ReconcileConvergesASecondRoute covers the path a drift repair takes: a
// source reports a new route and the next full reconcile serves it, with the panel
// untouched throughout.
func TestContract_ReconcileConvergesASecondRoute(t *testing.T) {
	live := startCaddy(t, "panel.test")
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

// TestContract_CaddyServesTheWebUIAndAPIOnOneHost is the check behind the frontend split.
// Caddy delivers the assets from disk and forwards only /api to Dinchy, which is what
// keeps the browser same-origin — and same-origin is what lets the session and CSRF
// cookies stay SameSite=Lax and keeps CORS uninvolved.
func TestContract_CaddyServesTheWebUIAndAPIOnOneHost(t *testing.T) {
	live := startCaddy(t, "panel.test")
	ctx := context.Background()

	webRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(webRoot, "index.html"),
		[]byte("<!doctype html><title>dinchy</title>"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(webRoot, "assets"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(webRoot, "assets", "app.js"),
		[]byte("console.log('app')"), 0o600))

	reconciler, err := caddy.NewReconciler(live.cfg, live.admin)
	require.NoError(t, err)
	reconciler.Register(caddy.NewStaticSource(caddy.PanelOwner,
		caddy.Route{
			Owner: caddy.PanelOwner, Host: live.cfg.PanelHost,
			PathPrefix: "/api", Upstream: echoUpstream(t, "api"),
		},
		caddy.Route{
			Owner: caddy.PanelOwner, Host: live.cfg.PanelHost,
			Serve: caddy.ServeModeFiles, Root: webRoot, FallbackPath: caddy.SPAFallbackPath,
		},
	))
	_, err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	// A real asset is served straight off disk, never touching Dinchy.
	status, body := live.getRaw(t, live.cfg.PanelHost, "/assets/app.js")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "console.log('app')")

	// A client-side route has no file behind it and must fall back to the document,
	// otherwise deep links 404 after a page reload.
	status, body = live.getRaw(t, live.cfg.PanelHost, "/settings/organization")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "<title>dinchy</title>", "an unmatched path must serve the SPA document")

	// The API prefix still reaches Dinchy, on the same hostname.
	status, body = live.getRaw(t, live.cfg.PanelHost, "/api/anything")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"upstream":"api"`, "/api must be proxied, not served from disk")
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
	live := startCaddy(t, "panel.test")
	ctx := context.Background()

	built, err := caddy.BuildConfig(live.cfg, []caddy.Route{{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: "not a host:8080",
	}})
	require.NoError(t, err)

	require.NoError(t, live.admin.LoadConfig(ctx, built), "Caddy does not check dial addresses at load time")

	status, _ := live.getRaw(t, live.cfg.PanelHost, "/")
	assert.Equal(t, http.StatusBadGateway, status, "the failure surfaces per request instead")
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

// routeHosts extracts the host every route Caddy is serving under Dinchy's server matches.
func (l liveCaddy) routeHosts(t *testing.T, running map[string]any) []string {
	t.Helper()
	apps, ok := running["apps"].(map[string]any)
	require.True(t, ok)
	httpApp, ok := apps["http"].(map[string]any)
	require.True(t, ok)
	servers, ok := httpApp["servers"].(map[string]any)
	require.True(t, ok)
	server, ok := servers[caddy.ServerName].(map[string]any)
	require.True(t, ok, "Caddy must keep the server under the name Dinchy owns")
	routes, ok := server["routes"].([]any)
	require.True(t, ok)

	hosts := make([]string, 0, len(routes))
	for _, entry := range routes {
		route, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		matches, ok := route["match"].([]any)
		if !ok || len(matches) == 0 {
			continue
		}
		match, ok := matches[0].(map[string]any)
		if !ok {
			continue
		}
		if matched, ok := match["host"].([]any); ok && len(matched) > 0 {
			if host, ok := matched[0].(string); ok {
				hosts = append(hosts, host)
			}
		}
	}
	return hosts
}
