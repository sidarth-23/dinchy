// Package caddy configures the Caddy reverse proxy that fronts Dinchy and every
// deployment it serves.
//
// Caddy is the only TLS terminator. Dinchy always listens plaintext on loopback; Caddy
// owns certificates, automatic HTTPS, and HSTS, and no certificate is ever loaded from
// disk — production obtains them over ACME, development signs them with Caddy's own local
// CA. Caddy is driven entirely through its JSON admin API, so there is no configuration
// file to keep in step.
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
// # One write, at startup
//
// [Reconciler.ReconcileAll] replaces the entire configuration, and it is the only write
// this package performs. It runs once, when the application starts. Nothing re-asserts it
// afterwards: on a self-hosted host the operator owns the running proxy, and a management
// plane that overwrites their changes on a timer is one they cannot work with.
//
// Replacing everything costs nothing while the only routes are the panel's and they never
// change at runtime. Once routes come and go, changing one must address that one —
// otherwise adding a domain drops an unrelated operator's web terminal — and that is when
// Caddy's per-object "@id" addressing earns its place here.
//
// The configuration Dinchy loads always contains an admin block, because a document
// omitting it would tear down the very endpoint used to push and leave no way to recover
// without hand-editing files.
//
// # Validation
//
// Almost none happens here. Caddy provisions the whole document on load and rolls back to
// the previous one if anything fails, so its rejection is the check — and a better error
// than one written twice. The exception is two routes claiming the same host and path,
// which Caddy accepts while leaving the loser unreachable behind the first terminal match.
//
// Caddy does not parse reverse-proxy dial addresses at load time either, so a malformed
// upstream loads cleanly and 502s per request. Anything composing a [Route] from user
// input has to screen that itself.
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
// The generated configuration is the only place forwarded headers are normalized. Dinchy
// has no forwarded-header middleware: it trusts X-Forwarded-For for the client address it
// records in audit rows, and relies on a loopback-only listener rather than a per-request
// check. That makes the header operations here load-bearing, which is why they are asserted
// against a live Caddy in the contract test rather than only against these structs.
//
// # Errors and logging
//
// Every function here returns its errors and logs none of them. Application startup is the
// one boundary that logs, for the boot reconcile.
package caddy
