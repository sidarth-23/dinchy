// Package caddy configures the shared Caddy edge that fronts Dinchy and every deployment it
// serves.
//
// The edge is not Dinchy's. One Caddy fronts every application on a host, owning its own
// listeners, ports, certificate storage and admin endpoint; Dinchy is one tenant on it and writes
// only the two objects it owns. deploy/caddy/ holds the edge itself.
//
// Caddy is the only TLS terminator. Dinchy always listens plaintext, and no certificate is ever
// loaded from disk — production obtains them over ACME, development signs them with Caddy's own
// local CA. Caddy is driven entirely through its JSON admin API, so there is no configuration file
// to keep in step.
//
// The edge reads nothing from disk on an application's behalf either. It is shared, so it can see
// none of any one application's files: every [Route] reverse-proxies, and a built asset is served
// by an upstream beside the application rather than by the edge.
//
// # Ownership
//
// This package owns the machinery: the route model, the translation into Caddy's
// configuration objects, the admin client, and reconciliation. It does not decide which
// routes exist. A module that owns public entrypoints implements [RouteSource] and
// registers it on the [Reconciler] at application wiring, the same way a feature
// registers an event subscriber. Compare internal/platform/email, where the Mailer owns
// delivery and the feature composes the message.
//
// Sources are pulled on reconcile rather than pushing changes inward. Pulling is what
// makes the desired configuration recomputable from stored state at any moment, which is
// the prerequisite for converging at startup and for repairing drift on request later.
//
// # Scoped writes
//
// [BuildContribution] produces the only two objects Dinchy writes, each addressed by a Caddy
// "@id" namespaced to this deployment (see [RouteObjectID], [TLSPolicyObjectID]):
//
//   - one route inside the edge's HTTP server, nesting every entrypoint as a subroute;
//   - one certificate automation policy covering the hosts those entrypoints answer on.
//
// Addressing one object each is what lets several applications share one edge. Replacing the whole
// document would drop every other tenant's routes — an unrelated operator's web terminal among
// them.
//
// A route is one object rather than several because that makes the write atomic, makes re-pushing
// it idempotent across a restart, and prunes an entrypoint a source has stopped reporting simply by
// leaving it out of the replacement. That last property is why there is no Remove operation.
//
// The route is terminal and matches on the union of the hosts it serves. Both matter: without the
// host matcher it would swallow every other tenant's traffic.
//
// # What the edge has to provide
//
// Caddy will not create a missing parent — it answers "invalid traversal path" — so the edge's base
// configuration must already contain the HTTP server named by DINCHY_CADDY_EDGE_SERVER with a
// routes array, and an automation policies array. That contract, and the reasoning behind each part
// of it, lives in deploy/caddy/README.md.
//
// # One write, at startup
//
// [Reconciler.ReconcileAll] runs once, when the application starts, and nothing re-asserts it
// afterwards: on a self-hosted host the operator owns the running proxy, and a management plane
// that overwrites their changes on a timer is one they cannot work with.
//
// The policy is written before the route, because a route whose certificate could not be arranged
// leaves the host failing its handshake, while a certificate no route uses yet breaks nothing.
//
// # Validation
//
// Almost none happens here. Caddy provisions the resulting whole configuration and rolls back to
// the previous one if anything fails, so its rejection is the check — and a better error than one
// written twice. A rejected write therefore leaves every other tenant serving.
//
// Two exceptions. Two of this deployment's own routes claiming the same host and path, which Caddy
// accepts while leaving the loser unreachable behind the first terminal match. And a host claimed
// by two different tenants, which nothing detects at all: each validates only its own slice, so
// first-in-array wins and the loser is silently unreachable. Finding that needs a read of the whole
// running configuration, which is a future concern rather than this one.
//
// Caddy does not parse reverse-proxy dial addresses at load time either, so a malformed
// upstream loads cleanly and 502s per request. Anything composing a [Route] from user
// input has to screen that itself.
//
// # What isolation does and does not mean
//
// It is isolation of configuration, not of connections. Caddy reloads on any config mutation, so
// one tenant's push can still cut another's long-lived connections. What it guarantees is that the
// other tenant's routes are still there afterwards.
//
// # Staying reachable
//
// A failed reconcile must never take the management plane down with it. The proxy being
// broken is exactly when an operator needs the interface that can fix it, so callers log
// the failure and continue serving. [Reconciler.Ping] is what readiness reports, so a
// proxy that recovers is seen without another push. Reserving the panel's hostname against
// other sources, and refusing a route that proxies back at the panel's own listener,
// belong to the same concern and come back with the first source that could violate either.
//
// # Forwarded headers
//
// The generated configuration is the only place forwarded headers are set, and it is one half of an
// invariant. Dinchy records X-Forwarded-For as the client address in audit rows; what makes that
// value trustworthy is the other half, DINCHY_TRUSTED_PROXIES, which stops the app honoring the
// header from any peer but the edge (see internal/transport/middleware.RequestInfo). The listener
// may face the edge's network, so the header alone proves nothing.
//
// That makes the header operations here load-bearing, which is why they are asserted against a live
// Caddy in the contract test rather than only against these structs.
//
// # Errors and logging
//
// Every function here returns its errors and logs none of them. Application startup is the
// one boundary that logs, for the boot reconcile.
package caddy
