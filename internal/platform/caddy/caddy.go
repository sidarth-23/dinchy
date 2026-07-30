package caddy

import (
	"context"

	"github.com/sidarth-23/dinchy/internal/foundation/transform"
)

// PanelOwner is the Owner of the routes serving Dinchy's own UI and API.
const PanelOwner = "panel"

// Route is one fully-resolved public entrypoint the edge serves. The owning module composes it;
// this package only translates it into Caddy's configuration and applies it. Compare email.Content:
// the payload type lives beside the seam that consumes it, so no feature has to import another
// feature to contribute one.
//
// Every Route reverse-proxies. The edge reads nothing from disk: it is shared between applications
// and has no access to any one application's files, so whatever serves a built asset is an upstream
// like any other.
type Route struct {
	// Owner names the RouteSource that contributed the Route. It gives conflicts a
	// blameable source and makes ordering deterministic.
	Owner string
	// Host is the hostname the site answers on.
	Host string
	// PathPrefix optionally narrows the Route to a path subtree. Empty serves the
	// whole host. On one host, prefixed Routes are matched before the unprefixed one.
	PathPrefix string
	// Upstream is the host:port the edge reverse-proxies to. It has to be reachable from the
	// edge, which is not the same thing as being an address the upstream binds.
	Upstream string
	// Headers are response headers Caddy sets on this Route, keyed by header name.
	Headers map[string]string
}

// RouteSource contributes the Routes its module owns. Sources are pulled on every
// reconcile rather than pushing changes, so the desired configuration always stays
// derivable from stored state — which is what makes boot convergence and drift repair
// possible at all.
type RouteSource interface {
	Name() string
	Routes(ctx context.Context) ([]Route, error)
}

// Result reports what a reconcile did.
type Result struct {
	// RouteCount is how many routes the applied route object carries.
	RouteCount int
	// Applied reports whether the reconcile pushed anything. It is false when Caddy
	// management is disabled, and when no source reported a route.
	Applied bool
}

// staticSource serves a fixed set of Routes.
type staticSource struct {
	name   string
	routes []Route
}

// NewStaticSource returns a RouteSource serving a fixed set of Routes, for
// entrypoints that come from configuration rather than from stored state.
func NewStaticSource(name string, routes ...Route) RouteSource {
	return staticSource{name: name, routes: routes}
}

func (s staticSource) Name() string { return s.name }

func (s staticSource) Routes(_ context.Context) ([]Route, error) { return s.routes, nil }

// Resolve canonicalizes user-supplied values so comparisons and rendering operate on
// normalized input. Hosts are trimmed and lowercased, which is what makes two Routes
// claiming one host compare equal whatever case they were written in.
func (r Route) Resolve() Route {
	resolved := r
	resolved.Host, _ = transform.Apply("trim,lower", r.Host)
	resolved.Upstream, _ = transform.Apply("trim,lower", r.Upstream)
	resolved.PathPrefix, _ = transform.Apply("trim", r.PathPrefix)
	resolved.Owner, _ = transform.Apply("trim", r.Owner)
	return resolved
}

// siteKey identifies the Caddy site a Route belongs to, so two Routes on one host
// merge rather than producing conflicting server blocks.
func (r Route) siteKey() string { return r.Host }
