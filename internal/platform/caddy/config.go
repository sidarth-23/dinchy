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

	// HTTPServer is one listening HTTP server.
	HTTPServer struct {
		Listen                []string              `json:"listen"`
		Routes                []ServerRoute         `json:"routes,omitempty"`
		TLSConnectionPolicies []TLSConnectionPolicy `json:"tls_connection_policies,omitempty"`
		AutomaticHTTPS        *AutomaticHTTPS       `json:"automatic_https,omitempty"`
	}

	// TLSConnectionPolicy picks which certificate answers a TLS handshake. Without a
	// policy naming the loaded certificate, Caddy has nothing to offer for an SNI it is
	// not managing automatically and aborts the handshake with an internal error.
	TLSConnectionPolicy struct {
		Match                *TLSPolicyMatch       `json:"match,omitempty"`
		CertificateSelection *CertificateSelection `json:"certificate_selection,omitempty"`
	}

	// TLSPolicyMatch selects the handshakes a policy applies to.
	TLSPolicyMatch struct {
		SNI []string `json:"sni,omitempty"`
	}

	// CertificateSelection narrows which loaded certificates a policy may serve.
	CertificateSelection struct {
		AnyTag []string `json:"any_tag,omitempty"`
	}

	// AutomaticHTTPS controls Caddy's implicit certificate and redirect behavior.
	AutomaticHTTPS struct {
		Disable          bool `json:"disable,omitempty"`
		DisableRedirects bool `json:"disable_redirects,omitempty"`
		DisableCerts     bool `json:"disable_certificates,omitempty"`
	}

	// ServerRoute is one route within a server. ID is Caddy's "@id" tag, which is what
	// makes a single route addressable at /id/<ID> for targeted updates.
	ServerRoute struct {
		ID       string         `json:"@id,omitempty"`
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

	// TLSApp configures certificate loading and automation.
	TLSApp struct {
		Certificates *TLSCertificates `json:"certificates,omitempty"`
		Automation   *TLSAutomation   `json:"automation,omitempty"`
	}

	// TLSCertificates lists certificates loaded from disk.
	TLSCertificates struct {
		LoadFiles []CertificateFile `json:"load_files,omitempty"`
	}

	// CertificateFile is one certificate and key pair on disk. Tags let a connection
	// policy select this certificate by name.
	CertificateFile struct {
		Certificate string   `json:"certificate"`
		Key         string   `json:"key"`
		Tags        []string `json:"tags,omitempty"`
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
		Module     string      `json:"module"`
		CA         string      `json:"ca,omitempty"`
		Email      string      `json:"email,omitempty"`
		Challenges *Challenges `json:"challenges,omitempty"`
	}

	// Challenges configures ACME challenge types.
	Challenges struct {
		DNS *DNSChallenge `json:"dns,omitempty"`
	}

	// DNSChallenge configures the DNS-01 challenge provider.
	DNSChallenge struct {
		Provider map[string]any `json:"provider,omitempty"`
	}
)

// RouteID returns the stable "@id" Caddy tags this Route with. It is derived from the
// owner, host and path so the same logical route always addresses the same object,
// which is what lets Dinchy update one route without replacing the whole config.
//
// The identifier carries no slash: the admin API addresses an object at /id/<id> and
// then treats anything further as a path into that object, so a slash inside the
// identifier would make the request ambiguous.
func RouteID(route Route) string {
	path := strings.ReplaceAll(strings.Trim(route.PathPrefix, "/"), "/", "_")
	if path == "" {
		path = "root"
	}
	return fmt.Sprintf("dinchy-%s-%s-%s", route.Owner, route.Host, path)
}

// BuildConfig translates the desired routes into a whole Caddy configuration document.
//
// The admin block is always present. POST /load replaces the entire configuration
// including that block, so a document that omitted it would tear down the very endpoint
// Dinchy uses to push, leaving no way to recover without editing files by hand.
func BuildConfig(cfg config.CaddyConfig, routes []Route) (Config, error) {
	ordered, err := orderRoutes(cfg, routes)
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
	tlsApp, policies := buildTLS(cfg, ordered)
	built.Apps.TLS = tlsApp
	server.TLSConnectionPolicies = policies
	applyAutomaticHTTPS(cfg, server)
	return built, nil
}

// applyAutomaticHTTPS disables Caddy's implicit certificate management and its
// HTTP-to-HTTPS redirect when Dinchy supplies the certificate itself.
//
// Redirects must be off in that mode because Caddy creates the redirect vhost on port
// 80 for every HTTPS site regardless of the site's own port. On a development machine
// where unprivileged ports start at 1024, binding 80 fails and Caddy exits.
func applyAutomaticHTTPS(cfg config.CaddyConfig, server *HTTPServer) {
	if !cfg.ServesOwnCertificate() {
		return
	}
	server.AutomaticHTTPS = &AutomaticHTTPS{DisableCerts: true, DisableRedirects: true}
}

// buildTLS loads explicit certificates or configures ACME automation, and returns the
// connection policies that let Caddy actually serve what was loaded.
//
// A loaded certificate is not enough on its own: for an SNI Caddy is not managing
// automatically, it needs a connection policy selecting that certificate, or it aborts
// the handshake with a TLS internal error. Each certificate is therefore tagged and
// matched by tag, mirroring what Caddy's own Caddyfile adapter produces.
func buildTLS(cfg config.CaddyConfig, routes []Route) (*TLSApp, []TLSConnectionPolicy) {
	app := &TLSApp{}
	var loadFiles []CertificateFile
	var policies []TLSConnectionPolicy
	tagByPair := map[string]string{}
	hostsByTag := map[string][]string{}

	for i := range routes {
		route := &routes[i]
		certFile, keyFile := route.CertFile, route.KeyFile
		if route.TLS != TLSModeFile {
			if !cfg.ServesOwnCertificate() || route.TLS == TLSModeAutomatic {
				continue
			}
			certFile, keyFile = cfg.CertFile, cfg.KeyFile
		}
		if certFile == "" || keyFile == "" {
			continue
		}
		pair := certFile + "\x00" + keyFile
		tag, ok := tagByPair[pair]
		if !ok {
			tag = "cert" + strconv.Itoa(len(tagByPair))
			tagByPair[pair] = tag
			loadFiles = append(loadFiles, CertificateFile{Certificate: certFile, Key: keyFile, Tags: []string{tag}})
		}
		hostsByTag[tag] = append(hostsByTag[tag], route.Host)
	}

	switch len(loadFiles) {
	case 0:
	case 1:
		// One certificate serves every name it covers, so the policy carries no SNI
		// match. That is what lets a route added later — a new deployment under the
		// wildcard — be served without also rewriting the connection policies.
		policies = append(policies, TLSConnectionPolicy{
			CertificateSelection: &CertificateSelection{AnyTag: []string{loadFiles[0].Tags[0]}},
		})
	default:
		// Several certificates need an SNI match each to be told apart, plus a trailing
		// catch-all so names none of them claim still complete a handshake.
		for _, file := range loadFiles {
			tag := file.Tags[0]
			hosts := hostsByTag[tag]
			slices.Sort(hosts)
			policies = append(policies, TLSConnectionPolicy{
				Match:                &TLSPolicyMatch{SNI: slices.Compact(hosts)},
				CertificateSelection: &CertificateSelection{AnyTag: []string{tag}},
			})
		}
		policies = append(policies, TLSConnectionPolicy{})
	}

	if len(loadFiles) > 0 {
		app.Certificates = &TLSCertificates{LoadFiles: loadFiles}
	}
	if automation := buildAutomationPolicies(cfg, routes); len(automation) > 0 {
		app.Automation = &TLSAutomation{Policies: automation}
	}
	if app.Certificates == nil && app.Automation == nil {
		return nil, policies
	}
	return app, policies
}

// buildAutomationPolicies emits one ACME policy per DNS provider in use, plus a
// default policy, so routes needing DNS-01 get it and the rest use the defaults.
func buildAutomationPolicies(cfg config.CaddyConfig, routes []Route) []AutomationPolicy {
	if cfg.ServesOwnCertificate() {
		return nil
	}
	byProvider := map[string][]string{}
	for i := range routes {
		route := &routes[i]
		if route.TLS == TLSModeFile {
			continue
		}
		byProvider[route.DNSProviderModule] = append(byProvider[route.DNSProviderModule], route.Host)
	}
	policies := make([]AutomationPolicy, 0, len(byProvider))
	for _, provider := range slices.Sorted(maps.Keys(byProvider)) {
		subjects := byProvider[provider]
		slices.Sort(subjects)
		issuer := Issuer{Module: "acme", CA: cfg.ACMECA, Email: cfg.ACMEEmail}
		if provider != "" {
			issuer.Challenges = &Challenges{DNS: &DNSChallenge{Provider: map[string]any{"name": dnsProviderName(provider)}}}
		}
		policies = append(policies, AutomationPolicy{Subjects: subjects, Issuers: []Issuer{issuer}})
	}
	return policies
}

// dnsProviderName reduces a Caddy DNS provider module ID to the provider name the
// acme issuer's challenge configuration expects.
func dnsProviderName(module string) string {
	return module[strings.LastIndex(module, ".")+1:]
}

// buildServerRoute translates one Route into a Caddy route object.
//
// The Host header is deliberately left alone. Caddy forwards the original by default,
// and the CORS origin check compares the request Origin against that host, so rewriting
// it to the upstream address would reject every cross-origin request.
func buildServerRoute(cfg config.CaddyConfig, route Route) ServerRoute {
	match := RouteMatch{Host: []string{route.Host}}
	if route.PathPrefix != "" {
		match.Path = []string{strings.TrimSuffix(route.PathPrefix, "/") + "/*"}
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

	return ServerRoute{ID: RouteID(route), Match: []RouteMatch{match}, Handle: handlers, Terminal: true}
}

// staticFileHandler serves Root from disk, optionally falling back to a single document
// for paths that match no file.
//
// The three steps are nested in a subroute because the fallback needs its own matcher and
// a top-level route carries only one: the root is set first so file matching resolves
// against it, then a rewrite redirects unmatched paths to the fallback, then the file
// server responds. Keeping it inside one route also keeps the route individually
// addressable at /id/<id>. This mirrors what Caddy's own Caddyfile adapter emits for
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
// order: the panel first, then exact hosts, then wildcards, and within one host the more
// specific path before the less specific.
//
// Caddy evaluates routes in array order and stops at the first terminal match, so path
// ordering is correctness rather than tidiness: an unprefixed route listed before a
// "/api" one on the same host would swallow every API request.
func orderRoutes(cfg config.CaddyConfig, routes []Route) ([]Route, error) {
	ordered := make([]Route, 0, len(routes))
	for i := range routes {
		ordered = append(ordered, routes[i].Resolve())
	}
	slices.SortStableFunc(ordered, func(a, b Route) int {
		if c := cmp.Compare(panelRank(a), panelRank(b)); c != 0 {
			return c
		}
		if c := cmp.Compare(wildcardRank(a), wildcardRank(b)); c != 0 {
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
	if err := validateRouteSet(cfg, ordered); err != nil {
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

func wildcardRank(route Route) int {
	if strings.HasPrefix(route.Host, "*.") {
		return 1
	}
	return 0
}

// validateRouteSet enforces the invariants that keep the proxy configuration coherent
// and, above all, keep the panel reachable.
func validateRouteSet(cfg config.CaddyConfig, routes []Route) error {
	// The panel contributes more than one route — the API and the web assets — so look
	// for the one that actually proxies, not merely the first the panel owns.
	panelUpstream := ""
	for i := range routes {
		if routes[i].Owner == PanelOwner && routes[i].Upstream != "" {
			panelUpstream = routes[i].Upstream
			break
		}
	}

	ownerByKey := map[string]string{}
	for i := range routes {
		route := &routes[i]
		if err := route.Validate(); err != nil {
			return err
		}
		if err := validatePanelReservation(cfg, *route); err != nil {
			return err
		}
		if err := validateNoUpstreamLoop(panelUpstream, *route); err != nil {
			return err
		}
		key := route.siteKey() + "\x00" + route.PathPrefix
		if existingOwner, ok := ownerByKey[key]; ok {
			return apperrors.Conflict(
				i18n.Msg(i18n.CodePlatformRoutingHostConflict),
				apperrors.WithHostname(apperrors.Hostname(route.Host)),
				apperrors.WithOwner(apperrors.Owner(existingOwner)),
				apperrors.WithConflictingOwner(apperrors.ConflictingOwner(route.Owner)),
				apperrors.WithCause(fmt.Errorf("host %q with path %q is claimed by both %q and %q", route.Host, route.PathPrefix, existingOwner, route.Owner)),
			)
		}
		ownerByKey[key] = route.Owner
	}
	return validateNoWildcardShadowing(routes)
}

// validatePanelReservation keeps any other source from claiming the panel's hostname.
// Without this a route can shadow the panel, and the operator loses the only interface
// that could remove the offending route.
func validatePanelReservation(cfg config.CaddyConfig, route Route) error {
	if route.Owner == PanelOwner || route.Host != cfg.PanelHost {
		return nil
	}
	return apperrors.Conflict(
		i18n.Msg(i18n.CodePlatformRoutingPanelHostReserved),
		apperrors.WithHostname(apperrors.Hostname(route.Host)),
		apperrors.WithOwner(apperrors.Owner(route.Owner)),
		apperrors.WithCause(fmt.Errorf("owner %q claimed the panel host %q", route.Owner, cfg.PanelHost)),
	)
}

// validateNoUpstreamLoop rejects a route that proxies back to the panel's own
// listener, which would make Caddy forward requests to Dinchy in a loop.
func validateNoUpstreamLoop(panelUpstream string, route Route) error {
	if route.Owner == PanelOwner || panelUpstream == "" || route.Upstream != panelUpstream {
		return nil
	}
	return apperrors.Conflict(
		i18n.Msg(i18n.CodePlatformRoutingUpstreamLoop),
		apperrors.WithHostname(apperrors.Hostname(route.Host)),
		apperrors.WithUpstream(apperrors.Upstream(route.Upstream)),
		apperrors.WithOwner(apperrors.Owner(route.Owner)),
		apperrors.WithCause(fmt.Errorf("owner %q pointed host %q at the panel upstream %q", route.Owner, route.Host, route.Upstream)),
	)
}

// validateNoWildcardShadowing rejects an exact host that a wildcard route would also
// match. Keeping every route's match set disjoint means Caddy's route ordering cannot
// change which route wins, so an incremental update can append safely.
func validateNoWildcardShadowing(routes []Route) error {
	for i := range routes {
		wildcard := &routes[i]
		suffix, ok := strings.CutPrefix(wildcard.Host, "*.")
		if !ok {
			continue
		}
		for j := range routes {
			exact := &routes[j]
			if exact.Host == wildcard.Host || !coveredByWildcard(exact.Host, suffix) {
				continue
			}
			return apperrors.Conflict(
				i18n.Msg(i18n.CodePlatformRoutingHostConflict),
				apperrors.WithHostname(apperrors.Hostname(exact.Host)),
				apperrors.WithOwner(apperrors.Owner(wildcard.Owner)),
				apperrors.WithConflictingOwner(apperrors.ConflictingOwner(exact.Owner)),
				apperrors.WithCause(fmt.Errorf("host %q is already covered by the wildcard %q", exact.Host, wildcard.Host)),
			)
		}
	}
	return nil
}

// coveredByWildcard reports whether host is the single extra label under suffix, which
// is exactly what a "*.suffix" wildcard certificate and matcher cover.
func coveredByWildcard(host, suffix string) bool {
	label, ok := strings.CutSuffix(host, "."+suffix)
	return ok && label != "" && !strings.Contains(label, ".")
}
