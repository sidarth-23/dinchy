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
	return cfg
}

// developmentConfig is the local-CA shape used locally.
func developmentConfig() config.CaddyConfig {
	cfg := config.DefaultCaddy()
	cfg.PanelHost = "localhost"
	cfg.TLSIssuer = config.TLSIssuerInternal
	cfg.HSTSMaxAge = 0
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

// contributionRoutes returns the entrypoints nested inside the one addressable route.
func contributionRoutes(t *testing.T, contribution caddy.Contribution) []caddy.ServerRoute {
	t.Helper()
	require.Len(t, contribution.Route.Handle, 1)
	require.Equal(t, "subroute", contribution.Route.Handle[0].Handler)
	return contribution.Route.Handle[0].Routes
}

// TestBuildContribution_CarriesTheTenantObjectIDs pins that both objects are addressable. Without
// an "@id" a push could only replace by position, and two tenants would overwrite each other.
func TestBuildContribution_CarriesTheTenantObjectIDs(t *testing.T) {
	cfg := productionConfig()
	cfg.Tenant = "alpha"

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	assert.Equal(t, "dinchy.alpha.routes", contribution.Route.ID)
	assert.Equal(t, "dinchy.alpha.tls", contribution.Policy.ID)
	assert.Equal(t, caddy.RouteObjectID("alpha"), contribution.Route.ID)
	assert.Equal(t, caddy.TLSPolicyObjectID("alpha"), contribution.Policy.ID)
}

// TestBuildContribution_OuterRouteMatchesOnlyOurHosts is the isolation property the whole shared
// edge rests on. The route is terminal, so a missing host matcher would make it swallow every other
// tenant's traffic — the edge would answer every request with this deployment's routes.
func TestBuildContribution_OuterRouteMatchesOnlyOurHosts(t *testing.T) {
	cfg := productionConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "other.example.com", Upstream: "127.0.0.1:32769"},
	})
	require.NoError(t, err)

	require.Len(t, contribution.Route.Match, 1)
	assert.Equal(t, []string{"other.example.com", cfg.PanelHost}, contribution.Route.Match[0].Host)
	assert.True(t, contribution.Route.Terminal)
}

// TestBuildContribution_PolicyNamesEverySubjectAndIsNeverACatchAll guards both directions. A policy
// with no subjects is a catch-all, and Caddy takes the first match, so it would take over issuance
// for every host on the edge. A host with no policy falls through to the edge's default issuer,
// which in development means a real ACME attempt for a name that cannot validate.
func TestBuildContribution_PolicyNamesEverySubjectAndIsNeverACatchAll(t *testing.T) {
	cfg := productionConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "other.example.com", Upstream: "127.0.0.1:32769"},
		{Owner: "deployments", Host: "other.example.com", PathPrefix: "/api", Upstream: "127.0.0.1:32770"},
	})
	require.NoError(t, err)

	assert.NotEmpty(t, contribution.Policy.Subjects, "a policy without subjects is a catch-all")
	assert.Equal(t, []string{"other.example.com", cfg.PanelHost}, contribution.Policy.Subjects,
		"every host is named exactly once")
	assert.Equal(t, contribution.Route.Match[0].Host, contribution.Policy.Subjects,
		"the hosts served and the hosts covered must be the same set")
}

func TestBuildContribution_NoRoutesProducesAnEmptyContribution(t *testing.T) {
	contribution, err := caddy.BuildContribution(productionConfig(), nil)
	require.NoError(t, err)

	assert.True(t, contribution.Empty())
	assert.Zero(t, contribution.RouteCount)
}

func TestBuildContribution_ProductionUsesACMEAndHSTS(t *testing.T) {
	cfg := productionConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	issuer := contribution.Policy.Issuers[0]
	assert.Equal(t, config.TLSIssuerACME, issuer.Module)
	assert.Equal(t, "ops@example.com", issuer.Email)
	assert.Equal(t, []string{"max-age=31536000; includeSubDomains"},
		headerSet(t, contributionRoutes(t, contribution))["Strict-Transport-Security"])
}

func TestBuildContribution_DevelopmentUsesTheLocalCA(t *testing.T) {
	cfg := developmentConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{panelRoute("localhost", "127.0.0.1:8080")})
	require.NoError(t, err)

	issuer := contribution.Policy.Issuers[0]
	assert.Equal(t, config.TLSIssuerInternal, issuer.Module)
	// Both are ACME notions; Issuer.CA means an authority identifier to the local module.
	assert.Empty(t, issuer.Email)
	assert.Empty(t, issuer.CA)
}

// TestBuildContribution_PathPrefixedRouteSortsBeforeTheCatchAll is correctness, not tidiness:
// Caddy stops at the first terminal match, so an unprefixed route listed first would
// swallow every API request on that host.
func TestBuildContribution_PathPrefixedRouteSortsBeforeTheCatchAll(t *testing.T) {
	cfg := productionConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{
		// Deliberately supplied catch-all first.
		{Owner: caddy.PanelOwner, Host: cfg.PanelHost, Upstream: "127.0.0.1:3000"},
		{Owner: caddy.PanelOwner, Host: cfg.PanelHost, PathPrefix: "/api", Upstream: "127.0.0.1:8080"},
	})
	require.NoError(t, err)

	routes := contributionRoutes(t, contribution)
	require.Len(t, routes, 2)
	assert.Equal(t, []string{"/api/*"}, routes[0].Match[0].Path, "the API prefix must be matched first")
	assert.Empty(t, routes[1].Match[0].Path, "the catch-all must come last")
}

func TestBuildContribution_LongerPathPrefixSortsFirst(t *testing.T) {
	cfg := productionConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "app.example.com", PathPrefix: "/api", Upstream: "127.0.0.1:1"},
		{Owner: "deployments", Host: "app.example.com", PathPrefix: "/api/v2", Upstream: "127.0.0.1:2"},
	})
	require.NoError(t, err)

	routes := contributionRoutes(t, contribution)
	paths := []string{}
	for _, route := range routes[1:] {
		paths = append(paths, route.Match[0].Path[0])
	}
	assert.Equal(t, []string{"/api/v2/*", "/api/*"}, paths, "the more specific prefix must win")
}

// TestBuildContribution_EveryRouteReverseProxies pins that the edge reads nothing from disk. It is
// shared between applications and can see none of their files, so a built asset is an upstream.
func TestBuildContribution_EveryRouteReverseProxies(t *testing.T) {
	cfg := productionConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "dinchy-web-static:8081"),
		{Owner: caddy.PanelOwner, Host: cfg.PanelHost, PathPrefix: "/api", Upstream: "dinchy-api:8080"},
	})
	require.NoError(t, err)

	for _, route := range contributionRoutes(t, contribution) {
		last := route.Handle[len(route.Handle)-1]
		assert.Equal(t, "reverse_proxy", last.Handler)
		require.Len(t, last.Upstreams, 1)
		assert.NotEmpty(t, last.Upstreams[0].Dial)
	}
}

func TestBuildContribution_OmitsHSTSWhenMaxAgeIsZero(t *testing.T) {
	cfg := developmentConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{panelRoute("localhost", "127.0.0.1:8080")})
	require.NoError(t, err)

	// An HSTS pin on localhost would force HTTPS on every other plaintext dev server
	// on the machine, and browsers make that hard to undo.
	assert.NotContains(t, headerSet(t, contributionRoutes(t, contribution)), "Strict-Transport-Security")
}

func TestBuildContribution_NormalizesForwardedHeaders(t *testing.T) {
	cfg := productionConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	proxy := proxyHandler(t, contributionRoutes(t, contribution))
	require.NotNil(t, proxy.Headers)
	require.NotNil(t, proxy.Headers.Request)
	assert.Equal(t, []string{"{http.request.remote.host}"}, proxy.Headers.Request.Set["X-Forwarded-For"])
	assert.Equal(t, []string{"{http.request.scheme}"}, proxy.Headers.Request.Set["X-Forwarded-Proto"])
	// Caddy never sets these, yet common Go middleware prefers them over
	// X-Forwarded-For, so a forged value would otherwise win.
	assert.ElementsMatch(t, []string{"X-Real-IP", "True-Client-IP", "Forwarded"}, proxy.Headers.Request.Delete)
}

func TestBuildContribution_IsDeterministicRegardlessOfInputOrder(t *testing.T) {
	cfg := productionConfig()
	routes := []caddy.Route{
		{Owner: "deployments", Host: "b.example.com", Upstream: "127.0.0.1:32770"},
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "a.example.com", Upstream: "127.0.0.1:32769"},
	}
	reversed := []caddy.Route{routes[2], routes[0], routes[1]}

	first, err := caddy.BuildContribution(cfg, routes)
	require.NoError(t, err)
	second, err := caddy.BuildContribution(cfg, reversed)
	require.NoError(t, err)

	assert.Equal(t, first, second, "the same route set must always produce the same objects")
	assert.Equal(t, cfg.PanelHost, routeHosts(contributionRoutes(t, first))[0], "the panel is always first")
}

func TestBuildContribution_RejectsDuplicateHostAndNamesTheOwner(t *testing.T) {
	cfg := productionConfig()

	// Caddy stops at the first terminal match, so the loser would be silently
	// unreachable rather than reported.
	_, err := caddy.BuildContribution(cfg, []caddy.Route{
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

func TestBuildContribution_PathPrefixRoutesShareAHost(t *testing.T) {
	cfg := productionConfig()

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "app.example.com", PathPrefix: "/api", Upstream: "127.0.0.1:32769"},
		{Owner: "deployments", Host: "app.example.com", PathPrefix: "/web", Upstream: "127.0.0.1:32770"},
	})
	require.NoError(t, err)

	routes := contributionRoutes(t, contribution)
	require.Len(t, routes, 3)
	assert.Equal(t, []string{"/api/*"}, routes[1].Match[0].Path)
	assert.Equal(t, []string{"/web/*"}, routes[2].Match[0].Path)
	assert.Equal(t, 3, contribution.RouteCount)
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

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	assert.Equal(t, []string{"max-age=172800"},
		headerSet(t, contributionRoutes(t, contribution))["Strict-Transport-Security"])
}

// headerSet returns the response headers the first route sets.
func headerSet(t *testing.T, routes []caddy.ServerRoute) map[string][]string {
	t.Helper()
	require.NotEmpty(t, routes)
	for _, handler := range routes[0].Handle {
		if handler.Handler == "headers" && handler.Response != nil {
			return handler.Response.Set
		}
	}
	return map[string][]string{}
}

// proxyHandler returns the reverse proxy handler of the first route.
func proxyHandler(t *testing.T, routes []caddy.ServerRoute) caddy.RouteHandler {
	t.Helper()
	require.NotEmpty(t, routes)
	for _, handler := range routes[0].Handle {
		if handler.Handler == "reverse_proxy" {
			return handler
		}
	}
	t.Fatal("no reverse_proxy handler found")
	return caddy.RouteHandler{}
}

// routeHosts returns the host each route matches, in the order Caddy evaluates them.
func routeHosts(routes []caddy.ServerRoute) []string {
	hosts := make([]string, 0, len(routes))
	for _, route := range routes {
		hosts = append(hosts, route.Match[0].Host[0])
	}
	return hosts
}
