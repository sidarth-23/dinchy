package caddy_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
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
	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
)

// These tests drive a real Caddy process. They exist because the defects this package is
// most likely to ship are semantic rather than structural: a field can be present, of the
// right type, and still fail to do anything. Both bugs found while building this package
// were of that kind — a certificate that loaded but was never served, and a wildcard
// hostname a TLS client refuses — and neither is expressible as a schema violation. Only
// a live handshake and a live request find them.
//
// They also catch drift: every Caddy release is an opportunity for a field name or module
// identifier in config.go to become wrong, which unit tests against our own structs
// cannot detect.
//
// Not covered here: the single-label wildcard rule (`*.localhost`). curl and OpenSSL
// reject a wildcard whose parent is one label, but Go's own certificate verification does
// not enforce it, so a Go client cannot observe the failure. The guard is Route.Validate,
// with unit coverage in config_test.go.

// caddyBinary locates a runnable Caddy, skipping the test when there is none so a machine
// without Caddy still passes the suite.
func caddyBinary(t *testing.T) string {
	t.Helper()
	candidates := []string{os.Getenv("DINCHY_CADDY_BINARY"), "tmp/caddy", "../../../tmp/caddy", "caddy"}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}
	t.Skip("no runnable Caddy binary found; set DINCHY_CADDY_BINARY or run `mise run caddy:build`")
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

// issueCertificate writes a self-signed certificate covering hosts, and returns the paths
// plus a pool that trusts it. Generating it here avoids depending on mkcert and keeps the
// test hermetic.
func issueCertificate(t *testing.T, dir string, hosts ...string) (certPath, keyPath string, roots *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: hosts[0]},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              hosts,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	require.NoError(t, os.WriteFile(certPath, certPEM, 0o600))

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))

	roots = x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(certPEM))
	return certPath, keyPath, roots
}

// liveCaddy is a running Caddy plus everything needed to talk to it and through it.
type liveCaddy struct {
	cfg    config.CaddyConfig
	admin  caddy.AdminClient
	client *http.Client
	port   int
}

// startCaddy launches an isolated Caddy and returns once its admin API answers.
func startCaddy(t *testing.T, hosts ...string) liveCaddy {
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

	certPath, keyPath, roots := issueCertificate(t, dir, hosts...)

	cmd := exec.Command(binary, "run", "--config", bootstrap) //nolint:gosec // path is test-controlled
	// Point Caddy's storage at the temp dir. Without this the test writes into the
	// developer's real certificate and ACME-account storage. --resume is deliberately
	// absent for the same reason: it would restore their dev configuration.
	cmd.Env = append(os.Environ(),
		"XDG_DATA_HOME="+filepath.Join(dir, "data"),
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
	cfg.PanelHost = hosts[0]
	cfg.AutomaticHTTPS = false
	cfg.CertFile, cfg.KeyFile = certPath, keyPath
	cfg.HSTSMaxAge = 0
	cfg.StoragePath = ""

	admin := caddy.NewAdminClient(cfg)
	ctx := context.Background()
	require.Eventually(t, func() bool { return admin.Ping(ctx) == nil }, 30*time.Second, 200*time.Millisecond,
		"caddy admin API never came up")

	return liveCaddy{cfg: cfg, admin: admin, client: httpsClient(roots, httpsPort), port: httpsPort}
}

// httpsClient dials every host at the loopback port Caddy listens on, so the test does not
// depend on how the machine resolves the hostnames being served.
func httpsClient(roots *x509.CertPool, port int) *http.Client {
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
	url := fmt.Sprintf("https://%s:%d%s", host, l.port, path)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)

	resp, err := l.client.Do(req)
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
	live := startCaddy(t, "panel.test", "*.apps.test")
	ctx := context.Background()
	panelUpstream := echoUpstream(t, "panel")

	reconciler, err := caddy.NewReconciler(live.cfg, live.admin, clock.System{}, caddy.ModuleSet{})
	require.NoError(t, err)
	reconciler.Register(caddy.NewStaticSource(caddy.PanelOwner, caddy.Route{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: panelUpstream,
		TLS: caddy.TLSModeFile, CertFile: live.cfg.CertFile, KeyFile: live.cfg.KeyFile,
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
		TLS: caddy.TLSModeFile, CertFile: live.cfg.CertFile, KeyFile: live.cfg.KeyFile,
	}})
	require.NoError(t, err)
	require.NoError(t, live.admin.LoadConfig(ctx, built))

	running := live.fetchConfig(t)

	// The admin endpoint must survive the load. A document omitting it would tear down
	// the very endpoint used to push, leaving no way back without editing files by hand.
	admin, ok := running["admin"].(map[string]any)
	require.True(t, ok, "the running configuration must keep an admin block")
	assert.Equal(t, live.cfg.AdminEndpoint, admin["listen"])

	// Caddy kept the route under our server name and preserved its identifier, which the
	// targeted-update path addresses at /id/<id>.
	assert.Contains(t, live.routeIDs(t, running), caddy.RouteID(caddy.Route{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost,
	}))
}

func TestContract_TargetedRouteChangesTakeEffect(t *testing.T) {
	live := startCaddy(t, "panel.test", "*.apps.test")
	ctx := context.Background()

	reconciler, err := caddy.NewReconciler(live.cfg, live.admin, clock.System{}, caddy.ModuleSet{})
	require.NoError(t, err)
	reconciler.Register(caddy.NewStaticSource(caddy.PanelOwner, caddy.Route{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: echoUpstream(t, "panel"),
		TLS: caddy.TLSModeFile, CertFile: live.cfg.CertFile, KeyFile: live.cfg.KeyFile,
	}))
	_, err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	deployment := caddy.Route{
		Owner: "deployments", Host: "whoami.apps.test", Upstream: echoUpstream(t, "deployment"),
		TLS: caddy.TLSModeFile, CertFile: live.cfg.CertFile, KeyFile: live.cfg.KeyFile,
	}

	require.NoError(t, reconciler.ApplyRoute(ctx, deployment))
	status, body := live.get(t, deployment.Host)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "deployment", body["upstream"], "a route added without a full reload must serve")

	// The panel keeps working, which is the point of updating one route at a time.
	_, panelBody := live.get(t, live.cfg.PanelHost)
	assert.Equal(t, "panel", panelBody["upstream"])

	require.NoError(t, reconciler.RemoveRoute(ctx, deployment))
	// Assert on the body, not the status: Caddy answers 200 with an empty body when no
	// route matches, so a status check would pass whether or not the route was removed.
	_, removedBody := live.get(t, deployment.Host)
	assert.NotEqual(t, "deployment", removedBody["upstream"], "the removed route must stop serving")
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

	certified := func(route caddy.Route) caddy.Route {
		route.TLS = caddy.TLSModeFile
		route.CertFile, route.KeyFile = live.cfg.CertFile, live.cfg.KeyFile
		return route
	}

	reconciler, err := caddy.NewReconciler(live.cfg, live.admin, clock.System{}, caddy.ModuleSet{})
	require.NoError(t, err)
	reconciler.Register(caddy.NewStaticSource(caddy.PanelOwner,
		certified(caddy.Route{
			Owner: caddy.PanelOwner, Host: live.cfg.PanelHost,
			PathPrefix: "/api", Upstream: echoUpstream(t, "api"),
		}),
		certified(caddy.Route{
			Owner: caddy.PanelOwner, Host: live.cfg.PanelHost,
			Serve: caddy.ServeModeFiles, Root: webRoot, FallbackPath: caddy.SPAFallbackPath,
		}),
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

func TestContract_CaddyRejectsAnInvalidUpstream(t *testing.T) {
	live := startCaddy(t, "panel.test")
	ctx := context.Background()

	// A syntactically valid document Caddy still refuses to provision, proving rejections
	// surface as errors rather than being silently accepted.
	built, err := caddy.BuildConfig(live.cfg, []caddy.Route{{
		Owner: caddy.PanelOwner, Host: live.cfg.PanelHost, Upstream: "127.0.0.1:8080",
		TLS: caddy.TLSModeFile, CertFile: live.cfg.CertFile, KeyFile: "/nonexistent/key.pem",
	}})
	require.NoError(t, err)

	err = live.admin.LoadConfig(ctx, built)

	require.Error(t, err, "a missing key file must be reported, not ignored")
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

// routeIDs extracts the "@id" of every route Caddy is serving under Dinchy's server.
func (l liveCaddy) routeIDs(t *testing.T, running map[string]any) []string {
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

	ids := make([]string, 0, len(routes))
	for _, entry := range routes {
		if route, ok := entry.(map[string]any); ok {
			if id, ok := route["@id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}
