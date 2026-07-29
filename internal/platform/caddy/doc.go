// Package caddy configures the Caddy reverse proxy that fronts Dinchy and every
// deployment it serves.
//
// Caddy is the only TLS terminator. Dinchy always listens plaintext on loopback; Caddy
// owns certificates, automatic HTTPS, and HSTS. Caddy is driven entirely through its
// JSON admin API, so the running configuration is the projection of Dinchy's own state
// and there is no configuration file to keep in step.
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
// Sources are pulled on every reconcile rather than pushing changes inward. Pulling is
// what makes the desired configuration recomputable from stored state at any moment,
// which is the prerequisite for converging at startup and for repairing drift.
//
// # Full load versus targeted updates
//
// [Reconciler.ReconcileAll] replaces the entire configuration and runs once at startup.
// [Reconciler.ApplyRoute] and [Reconciler.RemoveRoute] address a single route
// afterwards. The split is deliberate: replacing the whole configuration makes Caddy
// close active streaming connections, so a full reload on every routing change would
// drop one operator's web terminal or log stream because another added a domain. At
// startup there are no such connections, so converging in one call is free.
//
// Each route carries a stable "@id" derived from its owner, host and path (see
// [RouteID]), which is what makes a single route addressable at /id/<id> without
// knowing its position in the route array.
//
// Two invariants make the targeted path safe. The configuration Dinchy loads always
// contains an admin block, because a document omitting it would tear down the very
// endpoint used to push and leave no way to recover without hand-editing files. And
// every route's match set is kept disjoint, so appending a route can never change which
// route wins a request — an exact host that an existing wildcard would also match is
// rejected as a conflict rather than silently ordered.
//
// # Staying reachable
//
// A failed reconcile must never take the management plane down with it. The proxy being
// broken is exactly when an operator needs the interface that can fix it, so callers
// log a startup failure and continue serving; the recurring reconcile job retries until
// Caddy comes back. For the same reason no route may claim the panel's hostname, and no
// route may proxy back to the panel's own listener.
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
// Every function here returns its errors and logs none of them. The two boundaries that
// log are application startup, for the boot reconcile, and the scheduler's error
// listener, for the recurring job.
package caddy
