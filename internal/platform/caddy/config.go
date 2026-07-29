package caddy

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// ServerName is the key of the HTTP server Dinchy owns inside Caddy's configuration.
// Everything Dinchy manages lives under it, so an operator's own servers are left alone.
const ServerName = "dinchy"

// Caddy's configuration objects, modeled as plain structs rather than importing
// Caddy's own packages: the management plane keeps a small dependency tree, and the
// wire format is a stable, documented API.
type (
	// Config is a whole Caddy configuration document.
	Config struct {
		Admin   *AdminConfig   `json:"admin,omitempty"`
		Storage *StorageConfig `json:"storage,omitempty"`
		Apps    *AppsConfig    `json:"apps,omitempty"`
	}

	// AdminConfig configures the admin API endpoint Dinchy drives.
	AdminConfig struct {
		Listen string `json:"listen,omitempty"`
	}

	// StorageConfig pins where Caddy keeps certificates and ACME account keys.
	StorageConfig struct {
		Module string `json:"module"`
		Root   string `json:"root,omitempty"`
	}

	// AppsConfig holds the Caddy apps Dinchy configures.
	AppsConfig struct {
		HTTP *HTTPApp `json:"http,omitempty"`
		TLS  *TLSApp  `json:"tls,omitempty"`
	}

	// HTTPApp is Caddy's HTTP app.
	HTTPApp struct {
		HTTPPort  int                    `json:"http_port,omitempty"`
		HTTPSPort int                    `json:"https_port,omitempty"`
		Servers   map[string]*HTTPServer `json:"servers,omitempty"`
	}

	// HTTPServer is one listening HTTP server. It carries no TLS connection policy:
	// Caddy adds one itself for a server that listens only on the HTTPS port, and every
	// certificate here is one Caddy manages.
	HTTPServer struct {
		Listen         []string        `json:"listen"`
		Routes         []ServerRoute   `json:"routes,omitempty"`
		AutomaticHTTPS *AutomaticHTTPS `json:"automatic_https,omitempty"`
	}

	// AutomaticHTTPS controls Caddy's implicit HTTP-to-HTTPS redirect.
	AutomaticHTTPS struct {
		DisableRedirects bool `json:"disable_redirects,omitempty"`
	}

	// ServerRoute is one route within a server.
	ServerRoute struct {
		Match    []RouteMatch   `json:"match,omitempty"`
		Handle   []RouteHandler `json:"handle,omitempty"`
		Terminal bool           `json:"terminal,omitempty"`
	}

	// RouteMatch selects the requests a route handles.
	RouteMatch struct {
		Host []string     `json:"host,omitempty"`
		Path []string     `json:"path,omitempty"`
		File *FileMatcher `json:"file,omitempty"`
	}

	// FileMatcher matches when one of TryFiles exists on disk under the current root.
	// It is what lets a single-page application fall back to its index document.
	FileMatcher struct {
		TryFiles []string `json:"try_files,omitempty"`
	}

	// RouteHandler is one handler in a route's chain.
	RouteHandler struct {
		Handler   string        `json:"handler"`
		Response  *HeaderOps    `json:"response,omitempty"`
		Upstreams []Upstream    `json:"upstreams,omitempty"`
		Headers   *ProxyHeaders `json:"headers,omitempty"`
		// Root is the filesystem root, set by the "vars" handler before file matching.
		Root string `json:"root,omitempty"`
		// URI is the rewritten request URI, used by the "rewrite" handler.
		URI string `json:"uri,omitempty"`
		// Routes are the nested routes of a "subroute" handler, which is how per-path
		// matching is expressed inside one addressable top-level route.
		Routes []ServerRoute `json:"routes,omitempty"`
	}

	// HeaderOps sets or deletes headers.
	HeaderOps struct {
		Set    map[string][]string `json:"set,omitempty"`
		Delete []string            `json:"delete,omitempty"`
	}

	// ProxyHeaders rewrites headers on the proxied request.
	ProxyHeaders struct {
		Request *HeaderOps `json:"request,omitempty"`
	}

	// Upstream is one backend address.
	Upstream struct {
		Dial string `json:"dial"`
	}

	// TLSApp configures certificate automation.
	TLSApp struct {
		Automation *TLSAutomation `json:"automation,omitempty"`
	}

	// TLSAutomation holds the certificate automation policies.
	TLSAutomation struct {
		Policies []AutomationPolicy `json:"policies,omitempty"`
	}

	// AutomationPolicy governs how certificates for Subjects are obtained.
	AutomationPolicy struct {
		Subjects []string `json:"subjects,omitempty"`
		Issuers  []Issuer `json:"issuers,omitempty"`
	}

	// Issuer is a certificate issuer module configuration.
	Issuer struct {
		Module string `json:"module"`
		CA     string `json:"ca,omitempty"`
		Email  string `json:"email,omitempty"`
	}
)

// BuildConfig translates the desired routes into a whole Caddy configuration document.
//
// The admin block is always present. POST /load replaces the entire configuration
// including that block, so a document that omitted it would tear down the very endpoint
// Dinchy uses to push, leaving no way to recover without editing files by hand.
func BuildConfig(cfg config.CaddyConfig, routes []Route) (Config, error) {
	ordered, err := orderRoutes(routes)
	if err != nil {
		return Config{}, err
	}

	serverRoutes := make([]ServerRoute, 0, len(ordered))
	for i := range ordered {
		serverRoutes = append(serverRoutes, buildServerRoute(cfg, ordered[i]))
	}

	built := Config{
		Admin: &AdminConfig{Listen: cfg.AdminEndpoint},
		Apps: &AppsConfig{
			HTTP: &HTTPApp{
				HTTPSPort: int(cfg.HTTPSPort),
				Servers: map[string]*HTTPServer{
					ServerName: {
						Listen: []string{":" + strconv.Itoa(int(cfg.HTTPSPort))},
						Routes: serverRoutes,
					},
				},
			},
		},
	}
	if cfg.StoragePath != "" {
		built.Storage = &StorageConfig{Module: "file_system", Root: cfg.StoragePath}
	}
	server := built.Apps.HTTP.Servers[ServerName]
	built.Apps.TLS = buildTLS(cfg, ordered)
	if cfg.UsesLocalCA() {
		// Caddy creates the redirect vhost on port 80 for every HTTPS site regardless of
		// the site's own port. On a development machine where unprivileged ports start at
		// 1024, binding 80 fails and Caddy rejects the whole configuration.
		server.AutomaticHTTPS = &AutomaticHTTPS{DisableRedirects: true}
	}
	return built, nil
}

// buildTLS configures certificate automation for every host Dinchy serves. Nothing is
// loaded from disk and no connection policy is emitted: every certificate here is one
// Caddy manages, and Caddy adds the policy itself for a server that listens only on the
// HTTPS port.
func buildTLS(cfg config.CaddyConfig, routes []Route) *TLSApp {
	subjects := make([]string, 0, len(routes))
	for i := range routes {
		subjects = append(subjects, routes[i].Host)
	}
	if len(subjects) == 0 {
		return nil
	}
	slices.Sort(subjects)

	// The local CA takes no contact address or directory URL — both are ACME notions, and
	// Issuer.CA means an authority identifier to the internal module, not a directory.
	issuer := Issuer{Module: config.TLSIssuerInternal}
	if !cfg.UsesLocalCA() {
		issuer = Issuer{Module: config.TLSIssuerACME, CA: cfg.ACMECA, Email: cfg.ACMEEmail}
	}
	return &TLSApp{Automation: &TLSAutomation{Policies: []AutomationPolicy{{
		Subjects: slices.Compact(subjects),
		Issuers:  []Issuer{issuer},
	}}}}
}

// matcherPath renders a PathPrefix as the Caddy path matcher it becomes, or empty for a
// route that serves the whole host. A trailing slash is insignificant, so "/api" and
// "/api/" produce the same matcher.
func matcherPath(pathPrefix string) string {
	if pathPrefix == "" {
		return ""
	}
	return strings.TrimSuffix(pathPrefix, "/") + "/*"
}

// buildServerRoute translates one Route into a Caddy route object.
//
// The Host header is deliberately left alone. Caddy forwards the original by default,
// and the CORS origin check compares the request Origin against that host, so rewriting
// it to the upstream address would reject every cross-origin request.
func buildServerRoute(cfg config.CaddyConfig, route Route) ServerRoute {
	match := RouteMatch{Host: []string{route.Host}}
	if path := matcherPath(route.PathPrefix); path != "" {
		match.Path = []string{path}
	}

	var handlers []RouteHandler
	if response := buildResponseHeaders(cfg, route); response != nil {
		handlers = append(handlers, RouteHandler{Handler: "headers", Response: response})
	}
	if route.ServesFiles() {
		handlers = append(handlers, staticFileHandler(route))
	} else {
		handlers = append(handlers, RouteHandler{
			Handler:   "reverse_proxy",
			Upstreams: []Upstream{{Dial: route.Upstream}},
			Headers:   &ProxyHeaders{Request: forwardedHeaderOps()},
		})
	}

	return ServerRoute{Match: []RouteMatch{match}, Handle: handlers, Terminal: true}
}

// staticFileHandler serves Root from disk, optionally falling back to a single document
// for paths that match no file.
//
// The three steps are nested in a subroute because the fallback needs its own matcher and
// a top-level route carries only one: the root is set first so file matching resolves
// against it, then a rewrite redirects unmatched paths to the fallback, then the file
// server responds. This mirrors what Caddy's own Caddyfile adapter emits for
// `root` + `try_files` + `file_server`, verified with `caddy adapt`.
func staticFileHandler(route Route) RouteHandler {
	nested := []ServerRoute{
		{Handle: []RouteHandler{{Handler: "vars", Root: route.Root}}},
	}
	if route.FallbackPath != "" {
		nested = append(nested, ServerRoute{
			Match: []RouteMatch{{File: &FileMatcher{
				TryFiles: []string{"{http.request.uri.path}", route.FallbackPath},
			}}},
			Handle: []RouteHandler{{Handler: "rewrite", URI: "{http.matchers.file.relative}"}},
		})
	}
	nested = append(nested, ServerRoute{Handle: []RouteHandler{{Handler: "file_server"}}})
	return RouteHandler{Handler: "subroute", Routes: nested}
}

// forwardedHeaderOps normalizes the forwarded headers reaching every upstream.
//
// This is the only place these headers are handled: Dinchy carries no forwarded-header
// middleware of its own, so nothing downstream would notice if this regressed. The
// contract test in this package asserts the result against a live Caddy for that reason.
//
// X-Forwarded-For and X-Forwarded-Proto are set from the connection, replacing rather than
// appending, so a client cannot forge either. X-Forwarded-For is what Dinchy records as the
// client address in audit rows. X-Real-IP, True-Client-IP and Forwarded are deleted because
// Caddy does not manage them — it passes them through untouched — while common Go
// middleware prefers the first two over X-Forwarded-For, so a forged value would win.
func forwardedHeaderOps() *HeaderOps {
	return &HeaderOps{
		Set: map[string][]string{
			"X-Forwarded-For":   {"{http.request.remote.host}"},
			"X-Forwarded-Proto": {"{http.request.scheme}"},
			"X-Forwarded-Host":  {"{http.request.host}"},
		},
		Delete: []string{"X-Real-IP", "True-Client-IP", "Forwarded"},
	}
}

// buildResponseHeaders merges the Route's own headers with HSTS.
func buildResponseHeaders(cfg config.CaddyConfig, route Route) *HeaderOps {
	set := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(route.Headers)) {
		set[name] = []string{route.Headers[name]}
	}
	if value := hstsValue(cfg); value != "" {
		set["Strict-Transport-Security"] = []string{value}
	}
	if len(set) == 0 {
		return nil
	}
	return &HeaderOps{Set: set}
}

// hstsValue renders Strict-Transport-Security, or empty to omit the header.
//
// A zero max-age omits it, which is the right default for local development: pinning
// HSTS on localhost would force HTTPS on every other plaintext server on the same host,
// and browsers make that hard to undo.
func hstsValue(cfg config.CaddyConfig) string {
	if cfg.HSTSMaxAge <= 0 {
		return ""
	}
	value := "max-age=" + strconv.Itoa(int(cfg.HSTSMaxAge.Seconds()))
	if cfg.HSTSIncludeSubdomains {
		value += "; includeSubDomains"
	}
	return value
}

// orderRoutes validates the route set as a whole and returns it in a deterministic
// order: the panel first, then by host, and within one host the more specific path
// before the less specific.
//
// Caddy evaluates routes in array order and stops at the first terminal match, so path
// ordering is correctness rather than tidiness: an unprefixed route listed before a
// "/api" one on the same host would swallow every API request.
func orderRoutes(routes []Route) ([]Route, error) {
	ordered := make([]Route, 0, len(routes))
	for i := range routes {
		ordered = append(ordered, routes[i].Resolve())
	}
	slices.SortStableFunc(ordered, func(a, b Route) int {
		if c := cmp.Compare(panelRank(a), panelRank(b)); c != 0 {
			return c
		}
		return cmp.Or(
			cmp.Compare(a.Host, b.Host),
			// Longest prefix first, so the unprefixed catch-all sorts last.
			cmp.Compare(len(b.PathPrefix), len(a.PathPrefix)),
			cmp.Compare(a.PathPrefix, b.PathPrefix),
			cmp.Compare(a.Owner, b.Owner),
		)
	})
	if err := validateRouteSet(ordered); err != nil {
		return nil, err
	}
	return ordered, nil
}

func panelRank(route Route) int {
	if route.Owner == PanelOwner {
		return 0
	}
	return 1
}

// validateRouteSet rejects two routes claiming the same host and path.
//
// This is the only check Dinchy makes. Everything else a malformed route can be wrong
// about, Caddy reports when the configuration is loaded — it provisions before swapping
// and rolls back on failure, and that rejection reaches the caller as config_rejected.
// A duplicate is the exception: Caddy accepts it and stops at the first terminal match,
// leaving the losing route silently unreachable.
func validateRouteSet(routes []Route) error {
	ownerByKey := map[string]string{}
	for i := range routes {
		route := &routes[i]
		// Keyed on the matcher Caddy will actually see, so "/api" and "/api/" collide
		// here the same way they would collide in the running configuration.
		key := route.siteKey() + "\x00" + matcherPath(route.PathPrefix)
		if existingOwner, ok := ownerByKey[key]; ok {
			return apperrors.Conflict(
				i18n.Msg(i18n.CodePlatformRoutingHostConflict),
				apperrors.WithHostname(apperrors.Hostname(route.Host)),
				apperrors.WithOwner(apperrors.Owner(route.Owner)),
				apperrors.WithCause(fmt.Errorf("host %q with path %q is claimed by both %q and %q", route.Host, route.PathPrefix, existingOwner, route.Owner)),
			)
		}
		ownerByKey[key] = route.Owner
	}
	return nil
}
