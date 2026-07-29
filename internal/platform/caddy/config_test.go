package caddy_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
)

// productionConfig is the ACME-managed shape used in deployment.
func productionConfig() config.CaddyConfig {
	cfg := config.DefaultCaddy()
	cfg.PanelHost = "panel.example.com"
	cfg.ACMEEmail = "ops@example.com"
	cfg.HTTPSPort = 443
	return cfg
}

// developmentConfig is the local-CA shape used locally.
func developmentConfig() config.CaddyConfig {
	cfg := config.DefaultCaddy()
	cfg.PanelHost = "localhost"
	cfg.HTTPSPort = 8443
	cfg.TLSIssuer = config.TLSIssuerInternal
	cfg.HSTSMaxAge = 0
	cfg.StoragePath = ""
	return cfg
}

func panelRoute(host, upstream string) caddy.Route {
	return caddy.Route{Owner: caddy.PanelOwner, Host: host, Upstream: upstream}
}

// assertCode reports the stable error code and HTTP status a caller would observe.
func assertCode(t *testing.T, err error, want i18n.Code, wantStatus int) {
	t.Helper()
	require.Error(t, err)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, want, appErr.Code())
	assert.Equal(t, wantStatus, appErr.Status())
}

func TestBuildConfig_AlwaysCarriesAdminBlock(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	// Losing the admin block would tear down the endpoint Dinchy pushes through, with
	// no way back except editing files by hand.
	require.NotNil(t, built.Admin)
	assert.Equal(t, cfg.AdminEndpoint, built.Admin.Listen)
}

func TestBuildConfig_ProductionUsesAutomaticHTTPSAndHSTS(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	server := built.Apps.HTTP.Servers[caddy.ServerName]
	require.NotNil(t, server)
	assert.Nil(t, server.AutomaticHTTPS, "the HTTP-to-HTTPS redirect stays on in production")
	require.NotNil(t, built.Apps.TLS)
	require.NotNil(t, built.Apps.TLS.Automation)
	require.Len(t, built.Apps.TLS.Automation.Policies, 1)
	issuer := built.Apps.TLS.Automation.Policies[0].Issuers[0]
	assert.Equal(t, config.TLSIssuerACME, issuer.Module)
	assert.Equal(t, "ops@example.com", issuer.Email)

	assert.Equal(t, []string{"max-age=31536000; includeSubDomains"}, headerSet(t, server)["Strict-Transport-Security"])
}

func TestBuildConfig_DevelopmentUsesTheLocalCAAndDisablesRedirects(t *testing.T) {
	cfg := developmentConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute("localhost", "127.0.0.1:8080")})
	require.NoError(t, err)

	server := built.Apps.HTTP.Servers[caddy.ServerName]
	// Caddy builds the HTTP-to-HTTPS redirect vhost on port 80 for every HTTPS site
	// regardless of its port, and an unprivileged dev process cannot bind 80. Leaving this
	// on makes Caddy reject the whole document with a 400.
	require.NotNil(t, server.AutomaticHTTPS)
	assert.True(t, server.AutomaticHTTPS.DisableRedirects)

	require.NotNil(t, built.Apps.TLS.Automation)
	require.Len(t, built.Apps.TLS.Automation.Policies, 1)
	issuer := built.Apps.TLS.Automation.Policies[0].Issuers[0]
	assert.Equal(t, config.TLSIssuerInternal, issuer.Module)
	// Both are ACME notions; Issuer.CA means an authority identifier to the local module.
	assert.Empty(t, issuer.Email)
	assert.Empty(t, issuer.CA)
	assert.Nil(t, built.Storage, "no storage override, so Caddy uses the path its unit pins")
}

// TestBuildConfig_EveryHostIsCoveredByOnePolicy pins that a route added later is served
// without rewriting anything: one automation policy names every host Dinchy serves, and no
// connection policy is emitted at all — Caddy adds its own for an HTTPS-only server.
func TestBuildConfig_EveryHostIsCoveredByOnePolicy(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "other.example.com", Upstream: "127.0.0.1:32769"},
	})
	require.NoError(t, err)

	policies := built.Apps.TLS.Automation.Policies
	require.Len(t, policies, 1)
	assert.Equal(t, []string{"other.example.com", cfg.PanelHost}, policies[0].Subjects)
}

// TestBuildConfig_PathPrefixedRouteSortsBeforeTheCatchAll is correctness, not tidiness:
// Caddy stops at the first terminal match, so an unprefixed route listed first would
// swallow every API request on that host.
func TestBuildConfig_PathPrefixedRouteSortsBeforeTheCatchAll(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{
		// Deliberately supplied catch-all first.
		{Owner: caddy.PanelOwner, Host: cfg.PanelHost, Serve: caddy.ServeModeFiles, Root: "/srv/web", FallbackPath: "/index.html"},
		{Owner: caddy.PanelOwner, Host: cfg.PanelHost, PathPrefix: "/api", Upstream: "127.0.0.1:8080"},
	})
	require.NoError(t, err)

	routes := built.Apps.HTTP.Servers[caddy.ServerName].Routes
	require.Len(t, routes, 2)
	assert.Equal(t, []string{"/api/*"}, routes[0].Match[0].Path, "the API prefix must be matched first")
	assert.Empty(t, routes[1].Match[0].Path, "the catch-all must come last")
}

func TestBuildConfig_LongerPathPrefixSortsFirst(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "app.example.com", PathPrefix: "/api", Upstream: "127.0.0.1:1"},
		{Owner: "deployments", Host: "app.example.com", PathPrefix: "/api/v2", Upstream: "127.0.0.1:2"},
	})
	require.NoError(t, err)

	routes := built.Apps.HTTP.Servers[caddy.ServerName].Routes
	paths := []string{}
	for _, route := range routes[1:] {
		paths = append(paths, route.Match[0].Path[0])
	}
	assert.Equal(t, []string{"/api/v2/*", "/api/*"}, paths, "the more specific prefix must win")
}

func TestBuildConfig_StaticFileRouteServesTheRootWithAnSPAFallback(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{{
		Owner: caddy.PanelOwner, Host: cfg.PanelHost,
		Serve: caddy.ServeModeFiles, Root: "/srv/web/dist", FallbackPath: caddy.SPAFallbackPath,
	}})
	require.NoError(t, err)

	handlers := built.Apps.HTTP.Servers[caddy.ServerName].Routes[0].Handle
	subroute := handlers[len(handlers)-1]
	require.Equal(t, "subroute", subroute.Handler, "file serving is nested so the fallback can carry its own matcher")
	require.Len(t, subroute.Routes, 3)

	// The root is set first, because the file matcher resolves paths against it.
	assert.Equal(t, "vars", subroute.Routes[0].Handle[0].Handler)
	assert.Equal(t, "/srv/web/dist", subroute.Routes[0].Handle[0].Root)

	// An unmatched path rewrites to the application document, so client-side routes work.
	require.NotNil(t, subroute.Routes[1].Match[0].File)
	assert.Equal(t, []string{"{http.request.uri.path}", "/index.html"}, subroute.Routes[1].Match[0].File.TryFiles)
	assert.Equal(t, "rewrite", subroute.Routes[1].Handle[0].Handler)

	assert.Equal(t, "file_server", subroute.Routes[2].Handle[0].Handler)
}

func TestBuildConfig_StaticFileRouteWithoutFallbackOmitsTheRewrite(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{{
		Owner: caddy.PanelOwner, Host: cfg.PanelHost, Serve: caddy.ServeModeFiles, Root: "/srv/static",
	}})
	require.NoError(t, err)

	handlers := built.Apps.HTTP.Servers[caddy.ServerName].Routes[0].Handle
	subroute := handlers[len(handlers)-1]
	require.Len(t, subroute.Routes, 2, "no fallback means no rewrite step")
	assert.Equal(t, "vars", subroute.Routes[0].Handle[0].Handler)
	assert.Equal(t, "file_server", subroute.Routes[1].Handle[0].Handler)
}

func TestBuildConfig_StoragePathOverrideIsEmitted(t *testing.T) {
	cfg := productionConfig()
	cfg.StoragePath = "/srv/caddy-data"

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	require.NotNil(t, built.Storage)
	assert.Equal(t, "file_system", built.Storage.Module)
	assert.Equal(t, "/srv/caddy-data", built.Storage.Root)
}

func TestBuildConfig_OmitsHSTSWhenMaxAgeIsZero(t *testing.T) {
	cfg := developmentConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute("localhost", "127.0.0.1:8080")})
	require.NoError(t, err)

	// An HSTS pin on localhost would force HTTPS on every other plaintext dev server
	// on the machine, and browsers make that hard to undo.
	server := built.Apps.HTTP.Servers[caddy.ServerName]
	assert.NotContains(t, headerSet(t, server), "Strict-Transport-Security")
}

func TestBuildConfig_NormalizesForwardedHeaders(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	proxy := proxyHandler(t, built.Apps.HTTP.Servers[caddy.ServerName])
	require.NotNil(t, proxy.Headers)
	require.NotNil(t, proxy.Headers.Request)
	assert.Equal(t, []string{"{http.request.remote.host}"}, proxy.Headers.Request.Set["X-Forwarded-For"])
	assert.Equal(t, []string{"{http.request.scheme}"}, proxy.Headers.Request.Set["X-Forwarded-Proto"])
	// Caddy never sets these, yet common Go middleware prefers them over
	// X-Forwarded-For, so a forged value would otherwise win.
	assert.ElementsMatch(t, []string{"X-Real-IP", "True-Client-IP", "Forwarded"}, proxy.Headers.Request.Delete)
}

func TestBuildConfig_IsDeterministicRegardlessOfInputOrder(t *testing.T) {
	cfg := productionConfig()
	routes := []caddy.Route{
		{Owner: "deployments", Host: "b.example.com", Upstream: "127.0.0.1:32770"},
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "a.example.com", Upstream: "127.0.0.1:32769"},
	}
	reversed := []caddy.Route{routes[2], routes[0], routes[1]}

	first, err := caddy.BuildConfig(cfg, routes)
	require.NoError(t, err)
	second, err := caddy.BuildConfig(cfg, reversed)
	require.NoError(t, err)

	assert.Equal(t, first, second, "the same route set must always produce the same configuration")
	hosts := routeHosts(first.Apps.HTTP.Servers[caddy.ServerName])
	assert.Equal(t, cfg.PanelHost, hosts[0], "the panel is always first")
}

func TestBuildConfig_RejectsDuplicateHostAndNamesTheOwner(t *testing.T) {
	cfg := productionConfig()

	// Caddy stops at the first terminal match, so the loser would be silently
	// unreachable rather than reported.
	_, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:32769"},
		{Owner: "imports", Host: "app.example.com", Upstream: "127.0.0.1:32770"},
	})

	assertCode(t, err, i18n.CodePlatformRoutingHostConflict, http.StatusConflict)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	meta := appErr.Meta()
	assert.Equal(t, "app.example.com", meta[string(apperrors.MetaKeyHostname)])
	assert.NotEmpty(t, meta[string(apperrors.MetaKeyOwner)])
}

func TestBuildConfig_PathPrefixRoutesShareAHost(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "app.example.com", PathPrefix: "/api", Upstream: "127.0.0.1:32769"},
		{Owner: "deployments", Host: "app.example.com", PathPrefix: "/web", Upstream: "127.0.0.1:32770"},
	})
	require.NoError(t, err)

	server := built.Apps.HTTP.Servers[caddy.ServerName]
	require.Len(t, server.Routes, 3)
	assert.Equal(t, []string{"/api/*"}, server.Routes[1].Match[0].Path)
	assert.Equal(t, []string{"/web/*"}, server.Routes[2].Match[0].Path)
}

func TestRoute_ResolveNormalizesCaseAndWhitespace(t *testing.T) {
	first := caddy.Route{Owner: "deployments", Host: "App.Example.COM", Upstream: "127.0.0.1:8081"}
	second := caddy.Route{Owner: "deployments", Host: "  app.example.com  ", Upstream: "127.0.0.1:8081"}

	assert.Equal(t, first.Resolve(), second.Resolve())
}

func TestHSTS_UsesConfiguredMaxAge(t *testing.T) {
	cfg := productionConfig()
	cfg.HSTSMaxAge = 48 * time.Hour
	cfg.HSTSIncludeSubdomains = false

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	server := built.Apps.HTTP.Servers[caddy.ServerName]
	assert.Equal(t, []string{"max-age=172800"}, headerSet(t, server)["Strict-Transport-Security"])
}

// headerSet returns the response headers the first route sets.
func headerSet(t *testing.T, server *caddy.HTTPServer) map[string][]string {
	t.Helper()
	require.NotEmpty(t, server.Routes)
	for _, handler := range server.Routes[0].Handle {
		if handler.Handler == "headers" && handler.Response != nil {
			return handler.Response.Set
		}
	}
	return map[string][]string{}
}

// proxyHandler returns the reverse proxy handler of the first route.
func proxyHandler(t *testing.T, server *caddy.HTTPServer) caddy.RouteHandler {
	t.Helper()
	require.NotEmpty(t, server.Routes)
	for _, handler := range server.Routes[0].Handle {
		if handler.Handler == "reverse_proxy" {
			return handler
		}
	}
	t.Fatal("no reverse_proxy handler found")
	return caddy.RouteHandler{}
}

// routeHosts returns the host each route matches, in the order Caddy evaluates them.
func routeHosts(server *caddy.HTTPServer) []string {
	hosts := make([]string, 0, len(server.Routes))
	for _, route := range server.Routes {
		hosts = append(hosts, route.Match[0].Host[0])
	}
	return hosts
}
