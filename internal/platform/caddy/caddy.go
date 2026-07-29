package caddy

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/foundation/transform"
)

// TLSMode selects how Caddy provisions the certificate for a Route's host.
type TLSMode string

const (
	// TLSModeDefault follows the environment default from config.CaddyConfig.
	TLSModeDefault TLSMode = ""
	// TLSModeAutomatic lets Caddy obtain and renew the certificate over ACME.
	TLSModeAutomatic TLSMode = "automatic"
	// TLSModeFile serves the certificate and key named on the Route.
	TLSModeFile TLSMode = "file"
)

// PanelOwner is the reserved Owner for the routes serving Dinchy's own UI and API.
// No other source may claim the panel host.
const PanelOwner = "panel"

// ServeMode selects what a Route does with a matching request.
type ServeMode string

const (
	// ServeModeProxy reverse-proxies to Upstream.
	ServeModeProxy ServeMode = ""
	// ServeModeFiles serves static files from Root, which is how the compiled web UI is
	// delivered: Caddy reads it from disk directly rather than proxying through Dinchy.
	ServeModeFiles ServeMode = "files"
)

// SPAFallbackPath is the document a Route with a FallbackPath serves for a path that
// matches no file, which is what makes client-side routes survive a page load.
const SPAFallbackPath = "/index.html"

// Route is one fully-resolved public entrypoint Caddy serves. The owning module
// composes it; this package only translates it into Caddy's configuration and applies
// it. Compare email.Content: the payload type lives beside the seam that consumes it,
// so no feature has to import another feature to contribute one.
type Route struct {
	// Owner names the RouteSource that contributed the Route. It gives conflicts a
	// blameable source and makes ordering deterministic.
	Owner string
	// Host is the hostname the site answers on. A leading "*." wildcard is allowed.
	Host string
	// PathPrefix optionally narrows the Route to a path subtree. Empty serves the
	// whole host. On one host, prefixed Routes are matched before the unprefixed one.
	PathPrefix string
	// Serve selects whether the Route proxies to Upstream or serves files from Root.
	Serve ServeMode
	// Upstream is the loopback host:port Caddy reverse-proxies to, when Serve is
	// ServeModeProxy.
	Upstream string
	// Root is the directory Caddy serves files from, when Serve is ServeModeFiles.
	Root string
	// FallbackPath is served when no file matches the request path, so a single-page
	// application's client-side routes survive a page load. Empty returns 404 instead.
	// Only meaningful when Serve is ServeModeFiles.
	FallbackPath string
	// TLS selects certificate provisioning for Host.
	TLS TLSMode
	// CertFile is the PEM certificate served when TLS is TLSModeFile.
	CertFile string
	// KeyFile is the PEM private key served when TLS is TLSModeFile.
	KeyFile string
	// DNSProviderModule names the Caddy DNS provider module this Route needs for
	// DNS-01 challenges, such as "dns.providers.cloudflare". Empty uses the default
	// challenges. A module that is not compiled into the running Caddy is rejected
	// before it can fail at issuance time.
	DNSProviderModule string
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
	// RouteCount is how many routes the applied configuration contains.
	RouteCount int
	// Reloaded reports whether the full configuration was pushed.
	Reloaded bool
}

// Status is a snapshot of reconcile health, surfaced to readiness reporting. A
// degraded proxy is reported as degraded and never as unready: the management plane
// must stay up so an operator can repair the routing that broke.
type Status struct {
	// LastAttemptAt is when a reconcile last ran.
	LastAttemptAt time.Time
	// LastSuccessAt is when a reconcile last succeeded.
	LastSuccessAt time.Time
	// LastError is the most recent failure message, empty when healthy.
	LastError string
	// RouteCount is how many routes were last applied successfully.
	RouteCount int
	// Degraded reports whether the last reconcile failed.
	Degraded bool
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

// headerNamePattern matches the RFC 7230 token grammar for a header field name.
var headerNamePattern = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)

// Resolve canonicalizes user-supplied values so comparisons and rendering operate on
// normalized input. Hosts are trimmed and lowercased, which is what makes the
// panel-host reservation case-insensitive.
func (r Route) Resolve() Route {
	resolved := r
	resolved.Host, _ = transform.Apply("trim,lower", r.Host)
	resolved.Upstream, _ = transform.Apply("trim,lower", r.Upstream)
	resolved.PathPrefix, _ = transform.Apply("trim", r.PathPrefix)
	resolved.Owner, _ = transform.Apply("trim", r.Owner)
	resolved.DNSProviderModule, _ = transform.Apply("trim", r.DNSProviderModule)
	resolved.Root, _ = transform.Apply("trim", r.Root)
	resolved.FallbackPath, _ = transform.Apply("trim", r.FallbackPath)
	return resolved
}

// ServesFiles reports whether the Route serves static files rather than proxying.
func (r Route) ServesFiles() bool { return r.Serve == ServeModeFiles }

// Validate reports whether the Route is well-formed enough to translate into Caddy
// configuration. It assumes Resolve has already run.
func (r Route) Validate() error {
	if err := r.validateHost(); err != nil {
		return err
	}
	if err := r.validateTarget(); err != nil {
		return err
	}
	if r.PathPrefix != "" && !strings.HasPrefix(r.PathPrefix, "/") {
		return apperrors.BadRequest(
			i18n.Msg(i18n.CodePlatformRoutingInvalidPath),
			apperrors.WithFieldName("PathPrefix"),
			apperrors.WithHostname(apperrors.Hostname(r.Host)),
			apperrors.WithCause(fmt.Errorf("path prefix %q does not start with a slash", r.PathPrefix)),
		)
	}
	return r.validateHeaders()
}

func (r Route) validateHost() error {
	invalid := func(reason error) error {
		return apperrors.BadRequest(
			i18n.Msg(i18n.CodePlatformRoutingInvalidHost),
			apperrors.WithFieldName("Host"),
			apperrors.WithHostname(apperrors.Hostname(r.Host)),
			apperrors.WithCause(reason),
		)
	}
	if r.Host == "" {
		return invalid(fmt.Errorf("host is empty"))
	}
	// A bare "*" or a portless catch-all would shadow every other site, including the
	// panel, so it is rejected outright. A "*.example.com" wildcard is fine: Caddy
	// prefers an exact-match site block over a wildcard one.
	if r.Host == "*" {
		return invalid(fmt.Errorf("host %q is a catch-all", r.Host))
	}
	labels := r.Host
	if after, ok := strings.CutPrefix(labels, "*."); ok {
		labels = after
		if labels == "" || strings.Contains(labels, "*") {
			return invalid(fmt.Errorf("host %q is not a single wildcard label", r.Host))
		}
		// A wildcard directly under a single-label parent is rejected by TLS clients,
		// which treat "*.localhost" the same as "*.com" and refuse to match it. Caddy
		// will serve such a certificate happily, so catching it here is the only way the
		// operator learns before every request to the host fails verification.
		if !strings.Contains(labels, ".") {
			return invalid(fmt.Errorf("host %q wildcards a single-label parent, which TLS clients reject", r.Host))
		}
	}
	if strings.Contains(labels, "*") {
		return invalid(fmt.Errorf("host %q places a wildcard outside the leading label", r.Host))
	}
	if strings.ContainsAny(r.Host, " \t\r\n:/") {
		return invalid(fmt.Errorf("host %q contains a disallowed character", r.Host))
	}
	for label := range strings.SplitSeq(labels, ".") {
		if label == "" {
			return invalid(fmt.Errorf("host %q has an empty label", r.Host))
		}
	}
	return nil
}

// validateTarget checks whichever of Upstream or Root the Route's serve mode uses, and
// rejects supplying both — that would silently ignore one of them.
func (r Route) validateTarget() error {
	if r.ServesFiles() {
		return r.validateRoot()
	}
	if r.Root != "" || r.FallbackPath != "" {
		return apperrors.BadRequest(
			i18n.Msg(i18n.CodePlatformRoutingInvalidUpstream),
			apperrors.WithFieldName("Root"),
			apperrors.WithHostname(apperrors.Hostname(r.Host)),
			apperrors.WithCause(fmt.Errorf("host %q proxies to an upstream but also sets a file root %q", r.Host, r.Root)),
		)
	}
	return r.validateUpstream()
}

// validateRoot checks a static-file Route.
func (r Route) validateRoot() error {
	invalid := func(reason error) error {
		return apperrors.BadRequest(
			i18n.Msg(i18n.CodePlatformRoutingInvalidPath),
			apperrors.WithFieldName("Root"),
			apperrors.WithHostname(apperrors.Hostname(r.Host)),
			apperrors.WithCause(reason),
		)
	}
	if r.Root == "" {
		return invalid(fmt.Errorf("host %q serves files but names no root directory", r.Host))
	}
	if r.Upstream != "" {
		return invalid(fmt.Errorf("host %q serves files but also names upstream %q", r.Host, r.Upstream))
	}
	if r.FallbackPath != "" && !strings.HasPrefix(r.FallbackPath, "/") {
		return invalid(fmt.Errorf("fallback path %q does not start with a slash", r.FallbackPath))
	}
	return nil
}

func (r Route) validateUpstream() error {
	invalid := func(reason error) error {
		return apperrors.BadRequest(
			i18n.Msg(i18n.CodePlatformRoutingInvalidUpstream),
			apperrors.WithFieldName("Upstream"),
			apperrors.WithUpstream(apperrors.Upstream(r.Upstream)),
			apperrors.WithCause(reason),
		)
	}
	host, port, err := net.SplitHostPort(r.Upstream)
	if err != nil {
		return invalid(fmt.Errorf("split upstream %q: %w", r.Upstream, err))
	}
	if host == "" {
		return invalid(fmt.Errorf("upstream %q has no host", r.Upstream))
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		return invalid(fmt.Errorf("upstream %q has an out-of-range port %q", r.Upstream, port))
	}
	return nil
}

func (r Route) validateHeaders() error {
	for name, value := range r.Headers {
		// A header cannot carry a newline: it would terminate the field early on the
		// wire, so this is a validation failure rather than an escaping problem.
		if !headerNamePattern.MatchString(name) || strings.ContainsAny(value, "\r\n") {
			return apperrors.BadRequest(
				i18n.Msg(i18n.CodePlatformRoutingInvalidHeader),
				apperrors.WithFieldName("Headers"),
				apperrors.WithHostname(apperrors.Hostname(r.Host)),
				apperrors.WithCause(fmt.Errorf("header %q with value %q is not allowed", name, value)),
			)
		}
	}
	return nil
}

// siteKey identifies the Caddy site a Route belongs to, so two Routes on one host
// merge rather than producing conflicting server blocks.
func (r Route) siteKey() string { return r.Host }
