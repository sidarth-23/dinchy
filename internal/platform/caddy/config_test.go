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

// developmentConfig is the mkcert shape used locally.
func developmentConfig() config.CaddyConfig {
	cfg := config.DefaultCaddy()
	cfg.PanelHost = "localhost"
	cfg.HTTPSPort = 8443
	cfg.AutomaticHTTPS = false
	cfg.CertFile = "deploy/certs/app.pem"
	cfg.KeyFile = "deploy/certs/app-key.pem"
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
	assert.Nil(t, server.AutomaticHTTPS, "automatic HTTPS stays on in production")
	require.NotNil(t, built.Apps.TLS)
	assert.Nil(t, built.Apps.TLS.Certificates, "ACME provides the certificate")
	require.NotNil(t, built.Apps.TLS.Automation)
	require.Len(t, built.Apps.TLS.Automation.Policies, 1)
	assert.Equal(t, "ops@example.com", built.Apps.TLS.Automation.Policies[0].Issuers[0].Email)

	assert.Equal(t, []string{"max-age=31536000; includeSubDomains"}, headerSet(t, server)["Strict-Transport-Security"])
}

func TestBuildConfig_DevelopmentServesMkcertCertificateAndDisablesRedirects(t *testing.T) {
	cfg := developmentConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute("localhost", "127.0.0.1:8080")})
	require.NoError(t, err)

	server := built.Apps.HTTP.Servers[caddy.ServerName]
	require.NotNil(t, server.AutomaticHTTPS)
	assert.True(t, server.AutomaticHTTPS.DisableCerts)
	// Caddy builds the HTTP-to-HTTPS redirect vhost on port 80 for every HTTPS site
	// regardless of its port, and an unprivileged dev process cannot bind 80.
	assert.True(t, server.AutomaticHTTPS.DisableRedirects)

	require.NotNil(t, built.Apps.TLS.Certificates)
	require.Len(t, built.Apps.TLS.Certificates.LoadFiles, 1)
	assert.Equal(t, "deploy/certs/app.pem", built.Apps.TLS.Certificates.LoadFiles[0].Certificate)
	assert.Nil(t, built.Apps.TLS.Automation, "no ACME automation when serving our own certificate")
	assert.Nil(t, built.Storage, "no storage override, so Caddy uses the path its unit pins")
}

// TestBuildConfig_LoadedCertificateGetsAConnectionPolicy pins a contract found by
// driving a real Caddy: loading a certificate is not enough. Without a connection
// policy selecting it, Caddy has nothing to offer for an SNI it is not managing
// automatically and aborts the handshake with a TLS internal error.
func TestBuildConfig_LoadedCertificateGetsAConnectionPolicy(t *testing.T) {
	cfg := developmentConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute("localhost", "127.0.0.1:8080")})
	require.NoError(t, err)

	certs := built.Apps.TLS.Certificates.LoadFiles
	require.Len(t, certs, 1)
	require.Len(t, certs[0].Tags, 1)

	policies := built.Apps.HTTP.Servers[caddy.ServerName].TLSConnectionPolicies
	require.Len(t, policies, 1)
	// One certificate carries no SNI match, so a route added later — a new deployment
	// under the wildcard — is served without rewriting the policies as well.
	assert.Nil(t, policies[0].Match)
	assert.Equal(t, certs[0].Tags, policies[0].CertificateSelection.AnyTag)
}

func TestBuildConfig_SeveralCertificatesGetSNIMatchedPolicies(t *testing.T) {
	cfg := developmentConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute("localhost", "127.0.0.1:8080"),
		{
			Owner: "deployments", Host: "other.example.com", Upstream: "127.0.0.1:32769",
			TLS: caddy.TLSModeFile, CertFile: "/certs/other.pem", KeyFile: "/certs/other-key.pem",
		},
	})
	require.NoError(t, err)

	policies := built.Apps.HTTP.Servers[caddy.ServerName].TLSConnectionPolicies
	// One policy per certificate so they can be told apart, plus a trailing catch-all so
	// a name none of them claims still completes a handshake.
	require.Len(t, policies, 3)
	assert.NotNil(t, policies[0].Match)
	assert.NotNil(t, policies[1].Match)
	assert.Nil(t, policies[2].Match)
	assert.Nil(t, policies[2].CertificateSelection)
}

func TestBuildConfig_AutomaticHTTPSEmitsNoConnectionPolicies(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{panelRoute(cfg.PanelHost, "127.0.0.1:8080")})
	require.NoError(t, err)

	// Caddy manages these names itself, so it already has a certificate to offer.
	assert.Empty(t, built.Apps.HTTP.Servers[caddy.ServerName].TLSConnectionPolicies)
}

func TestRouteID_ContainsNoSlash(t *testing.T) {
	// The admin API addresses an object at /id/<id> and treats anything further as a
	// path into it, so a slash in the identifier would make the request ambiguous.
	route := caddy.Route{Owner: "deployments", Host: "app.example.com", PathPrefix: "/api/v1", Upstream: "127.0.0.1:8081"}

	id := caddy.RouteID(route.Resolve())

	assert.NotContains(t, id, "/")
	assert.Equal(t, "dinchy-deployments-app.example.com-api_v1", id)
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

func TestRoute_ValidateServeModes(t *testing.T) {
	tests := []struct {
		name     string
		route    caddy.Route
		wantCode i18n.Code
	}{
		{
			name:     "files without a root",
			route:    caddy.Route{Owner: "panel", Host: "panel.example.com", Serve: caddy.ServeModeFiles},
			wantCode: i18n.CodePlatformRoutingInvalidPath,
		},
		{
			name: "files with an upstream as well",
			route: caddy.Route{
				Owner: "panel", Host: "panel.example.com", Serve: caddy.ServeModeFiles,
				Root: "/srv/web", Upstream: "127.0.0.1:8080",
			},
			wantCode: i18n.CodePlatformRoutingInvalidPath,
		},
		{
			name: "files with a relative fallback",
			route: caddy.Route{
				Owner: "panel", Host: "panel.example.com", Serve: caddy.ServeModeFiles,
				Root: "/srv/web", FallbackPath: "index.html",
			},
			wantCode: i18n.CodePlatformRoutingInvalidPath,
		},
		{
			name: "proxy that also names a root",
			route: caddy.Route{
				Owner: "panel", Host: "panel.example.com", Upstream: "127.0.0.1:8080", Root: "/srv/web",
			},
			wantCode: i18n.CodePlatformRoutingInvalidUpstream,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCode(t, tt.route.Resolve().Validate(), tt.wantCode, http.StatusBadRequest)
		})
	}
}

func TestRoute_ValidateAcceptsAFileRoute(t *testing.T) {
	route := caddy.Route{
		Owner: caddy.PanelOwner, Host: "panel.example.com",
		Serve: caddy.ServeModeFiles, Root: "/srv/web/dist", FallbackPath: caddy.SPAFallbackPath,
	}

	require.NoError(t, route.Resolve().Validate())
	assert.True(t, route.ServesFiles())
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
	ids := routeIDs(first.Apps.HTTP.Servers[caddy.ServerName])
	assert.Equal(t, "dinchy-panel-panel.example.com-root", ids[0], "the panel is always first")
}

func TestBuildConfig_RejectsRouteClaimingThePanelHost(t *testing.T) {
	cfg := productionConfig()

	_, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		// Shadowing the panel would remove the only interface able to undo it.
		{Owner: "deployments", Host: "PANEL.example.com", Upstream: "127.0.0.1:32769"},
	})

	assertCode(t, err, i18n.CodePlatformRoutingPanelHostReserved, http.StatusConflict)
}

func TestBuildConfig_RejectsDuplicateHostAndNamesBothOwners(t *testing.T) {
	cfg := productionConfig()

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
	assert.NotEmpty(t, meta[string(apperrors.MetaKeyConflictingOwner)])
}

func TestBuildConfig_RejectsExactHostShadowedByWildcard(t *testing.T) {
	cfg := productionConfig()

	// Overlapping match sets would make Caddy's route ordering decide the winner,
	// which an incremental append could then silently change.
	_, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "*.apps.example.com", Upstream: "127.0.0.1:32769"},
		{Owner: "imports", Host: "one.apps.example.com", Upstream: "127.0.0.1:32770"},
	})

	assertCode(t, err, i18n.CodePlatformRoutingHostConflict, http.StatusConflict)
}

func TestBuildConfig_AllowsWildcardBesideUnrelatedHosts(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "*.apps.example.com", Upstream: "127.0.0.1:32769"},
		{Owner: "deployments", Host: "other.example.com", Upstream: "127.0.0.1:32770"},
	})
	require.NoError(t, err)

	ids := routeIDs(built.Apps.HTTP.Servers[caddy.ServerName])
	assert.Equal(t, "dinchy-deployments-*.apps.example.com-root", ids[len(ids)-1], "wildcards sort last")
}

func TestBuildConfig_RejectsUpstreamPointingAtThePanel(t *testing.T) {
	cfg := productionConfig()

	_, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:8080"},
	})

	assertCode(t, err, i18n.CodePlatformRoutingUpstreamLoop, http.StatusConflict)
}

func TestBuildConfig_RejectsCatchAllHost(t *testing.T) {
	cfg := productionConfig()

	_, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "*", Upstream: "127.0.0.1:32769"},
	})

	assertCode(t, err, i18n.CodePlatformRoutingInvalidHost, http.StatusBadRequest)
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

func TestBuildConfig_GroupsACMEPoliciesByDNSProvider(t *testing.T) {
	cfg := productionConfig()

	built, err := caddy.BuildConfig(cfg, []caddy.Route{
		panelRoute(cfg.PanelHost, "127.0.0.1:8080"),
		{Owner: "deployments", Host: "nat.example.com", Upstream: "127.0.0.1:32769", DNSProviderModule: "dns.providers.cloudflare"},
	})
	require.NoError(t, err)

	policies := built.Apps.TLS.Automation.Policies
	require.Len(t, policies, 2)
	assert.Nil(t, policies[0].Issuers[0].Challenges, "the default policy uses the default challenges")
	require.NotNil(t, policies[1].Issuers[0].Challenges)
	assert.Equal(t, "cloudflare", policies[1].Issuers[0].Challenges.DNS.Provider["name"])
	assert.Equal(t, []string{"nat.example.com"}, policies[1].Subjects)
}

func TestRoute_Validate(t *testing.T) {
	tests := []struct {
		name       string
		route      caddy.Route
		wantCode   i18n.Code
		wantStatus int
	}{
		{
			name:       "upstream without a port",
			route:      caddy.Route{Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1"},
			wantCode:   i18n.CodePlatformRoutingInvalidUpstream,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "upstream port out of range",
			route:      caddy.Route{Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:70000"},
			wantCode:   i18n.CodePlatformRoutingInvalidUpstream,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "path prefix without a leading slash",
			route:      caddy.Route{Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:8081", PathPrefix: "api"},
			wantCode:   i18n.CodePlatformRoutingInvalidPath,
			wantStatus: http.StatusBadRequest,
		},
		{
			// TLS clients treat "*.localhost" like "*.com" and refuse to match it, so
			// Caddy would serve a certificate every request then rejects.
			name:       "wildcard over a single-label parent",
			route:      caddy.Route{Owner: "deployments", Host: "*.localhost", Upstream: "127.0.0.1:8081"},
			wantCode:   i18n.CodePlatformRoutingInvalidHost,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "wildcard outside the leading label",
			route:      caddy.Route{Owner: "deployments", Host: "app.*.example.com", Upstream: "127.0.0.1:8081"},
			wantCode:   i18n.CodePlatformRoutingInvalidHost,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "host carrying a port",
			route:      caddy.Route{Owner: "deployments", Host: "app.example.com:8443", Upstream: "127.0.0.1:8081"},
			wantCode:   i18n.CodePlatformRoutingInvalidHost,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "header value containing a newline",
			route: caddy.Route{
				Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:8081",
				Headers: map[string]string{"X-Test": "value\r\nInjected: yes"},
			},
			wantCode:   i18n.CodePlatformRoutingInvalidHeader,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "header name outside the token grammar",
			route: caddy.Route{
				Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:8081",
				Headers: map[string]string{"Bad Header": "value"},
			},
			wantCode:   i18n.CodePlatformRoutingInvalidHeader,
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCode(t, tt.route.Resolve().Validate(), tt.wantCode, tt.wantStatus)
		})
	}
}

func TestRoute_ValidateAcceptsWildcardAndPlainHosts(t *testing.T) {
	for _, host := range []string{"localhost", "app.example.com", "*.apps.example.com", "*.dinchy.localhost"} {
		t.Run(host, func(t *testing.T) {
			route := caddy.Route{Owner: "deployments", Host: host, Upstream: "127.0.0.1:8081"}
			require.NoError(t, route.Resolve().Validate())
		})
	}
}

func TestRouteID_IsStableAcrossCaseAndWhitespace(t *testing.T) {
	first := caddy.Route{Owner: "deployments", Host: "App.Example.COM", Upstream: "127.0.0.1:8081"}
	second := caddy.Route{Owner: "deployments", Host: "  app.example.com  ", Upstream: "127.0.0.1:8081"}

	assert.Equal(t, caddy.RouteID(first.Resolve()), caddy.RouteID(second.Resolve()))
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

func routeIDs(server *caddy.HTTPServer) []string {
	ids := make([]string, 0, len(server.Routes))
	for _, route := range server.Routes {
		ids = append(ids, route.ID)
	}
	return ids
}
