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

// objectIDPrefix namespaces every "@id" this package writes, so an object Dinchy owns is
// recognizable in a configuration it shares with other tenants.
const objectIDPrefix = "dinchy"

// RouteObjectID is the "@id" of the single route object carrying every entrypoint this
// deployment serves. It is namespaced by tenant so two deployments sharing one edge address
// their own route and never each other's.
func RouteObjectID(tenant string) string {
	return objectIDPrefix + "." + tenant + ".routes"
}

// TLSPolicyObjectID is the "@id" of the certificate automation policy covering this
// deployment's hosts, namespaced the same way RouteObjectID is.
func TLSPolicyObjectID(tenant string) string {
	return objectIDPrefix + "." + tenant + ".tls"
}

// Caddy's configuration objects, modeled as plain structs rather than importing
// Caddy's own packages: the management plane keeps a small dependency tree, and the
// wire format is a stable, documented API.
//
// Only the objects Dinchy writes are modeled. The edge owns the admin endpoint, the storage
// location, the ports and the servers, and nothing here can express them.
type (
	// ServerRoute is one route within a server. ID is set only on the one route this
	// deployment owns, which is what makes it addressable without a path.
	ServerRoute struct {
		ID       string         `json:"@id,omitempty"`
		Match    []RouteMatch   `json:"match,omitempty"`
		Handle   []RouteHandler `json:"handle,omitempty"`
		Terminal bool           `json:"terminal,omitempty"`
	}

	// RouteMatch selects the requests a route handles.
	RouteMatch struct {
		Host []string `json:"host,omitempty"`
		Path []string `json:"path,omitempty"`
	}

	// RouteHandler is one handler in a route's chain.
	RouteHandler struct {
		Handler   string        `json:"handler"`
		Response  *HeaderOps    `json:"response,omitempty"`
		Upstreams []Upstream    `json:"upstreams,omitempty"`
		Headers   *ProxyHeaders `json:"headers,omitempty"`
		// Routes are the nested routes of a "subroute" handler, which is how several
		// entrypoints are expressed inside one addressable top-level route.
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

	// AutomationPolicy governs how certificates for Subjects are obtained. ID makes this
	// deployment's policy addressable inside the edge's shared policy array.
	AutomationPolicy struct {
		ID       string   `json:"@id,omitempty"`
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

// Contribution is the slice of the edge's configuration this deployment owns: one addressable
// route carrying every entrypoint it serves, and one certificate automation policy covering
// their hosts. Nothing outside it is Dinchy's to write.
type Contribution struct {
	// Route carries every entrypoint, nested inside one addressable object.
	Route ServerRoute
	// Policy covers the hosts those entrypoints answer on.
	Policy AutomationPolicy
	// RouteCount is how many entrypoints Route nests, for reporting.
	RouteCount int
}

// Empty reports whether there is nothing to apply, which is what an empty route set produces.
func (c Contribution) Empty() bool { return c.RouteCount == 0 }

// BuildContribution translates the desired routes into the two objects Dinchy pushes.
//
// Every entrypoint is nested inside one addressable route rather than contributed as several
// top-level ones. That is what makes the write atomic, makes re-pushing it idempotent, and prunes
// a route a source has stopped reporting simply by leaving it out of the replacement.
//
// The outer route matches on the union of the hosts it serves, and that matcher is load-bearing: it
// is terminal, so without a host matcher it would swallow every other tenant's traffic on the
// shared edge.
func BuildContribution(cfg config.CaddyConfig, routes []Route) (Contribution, error) {
	ordered, err := orderRoutes(routes)
	if err != nil {
		return Contribution{}, err
	}
	if len(ordered) == 0 {
		return Contribution{}, nil
	}

	nested := make([]ServerRoute, 0, len(ordered))
	for i := range ordered {
		nested = append(nested, buildServerRoute(cfg, ordered[i]))
	}
	hosts := serviceHosts(ordered)

	return Contribution{
		Route: ServerRoute{
			ID:       RouteObjectID(cfg.Tenant),
			Match:    []RouteMatch{{Host: hosts}},
			Handle:   []RouteHandler{{Handler: "subroute", Routes: nested}},
			Terminal: true,
		},
		Policy:     buildPolicy(cfg, hosts),
		RouteCount: len(ordered),
	}, nil
}

// serviceHosts is the deduplicated, ordered set of hosts the routes answer on.
func serviceHosts(routes []Route) []string {
	hosts := make([]string, 0, len(routes))
	for i := range routes {
		hosts = append(hosts, routes[i].Host)
	}
	slices.Sort(hosts)
	return slices.Compact(hosts)
}

// buildPolicy configures certificate automation for every host Dinchy serves. No certificate is
// loaded from disk and no connection policy is emitted: every certificate here is one the edge
// manages, and the edge's server carries the connection policy.
//
// Subjects are always named. A policy without them is a catch-all, and Caddy takes the first
// policy that matches — so an unnamed one would take over issuance for every host on the edge,
// including other tenants'. The converse matters too: a host served by a route but named by no
// policy falls through to the edge's default issuer, which in development means a real ACME
// attempt for a name that cannot validate.
func buildPolicy(cfg config.CaddyConfig, hosts []string) AutomationPolicy {
	// The local CA takes no contact address or directory URL — both are ACME notions, and
	// Issuer.CA means an authority identifier to the internal module, not a directory.
	issuer := Issuer{Module: config.TLSIssuerInternal}
	if !cfg.UsesLocalCA() {
		issuer = Issuer{Module: config.TLSIssuerACME, CA: cfg.ACMECA, Email: cfg.ACMEEmail}
	}
	return AutomationPolicy{
		ID:       TLSPolicyObjectID(cfg.Tenant),
		Subjects: hosts,
		Issuers:  []Issuer{issuer},
	}
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
	handlers = append(handlers, RouteHandler{
		Handler:   "reverse_proxy",
		Upstreams: []Upstream{{Dial: route.Upstream}},
		Headers:   &ProxyHeaders{Request: forwardedHeaderOps()},
	})

	return ServerRoute{Match: []RouteMatch{match}, Handle: handlers, Terminal: true}
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
