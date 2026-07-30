# Dinchy domain context

Ubiquitous language for the codebase. Names here are the ones to use in code,
tests, and design discussion. Keep entries terse and intent-focused.

This file is domain facts, not rules of conduct: behavioral rules live in
`.rules`, and each package's usage is documented in its godoc (`go doc <pkg>`).

## Delivery and messaging

- **Mailer** (`internal/platform/email`) — the email delivery seam. It renders a
  resolved `Content` into the shared branded layout and enqueues it for durable
  delivery through the job queue. It is copy-agnostic and link-agnostic: it
  selects no message text and builds no URLs. A feature reaches delivery only
  through `Mailer.Send(ctx, to, Content)`; the transport (`Sender`: SMTP or
  Noop) sits behind it and is never touched by callers.

- **Content** (`internal/platform/email`) — a fully-resolved email ready to
  render: subject, heading, body, call-to-action label and URL, and footer. The
  owning feature composes it (resolving copy and building the link); the Mailer
  only renders and delivers it. Contrast with **Message**, the low-level
  transport payload (`To`, `Subject`, `Text`, `HTML`) that the `Sender` delivers.

- **Links** (`internal/config`) — the single source for outbound email
  call-to-action links: the public base URL plus the well-known frontend route
  paths (`AcceptInvitationPath`, `ResetPasswordPath`). Features read `Links` to
  assemble a `Content.CTAURL`; base URL and route paths live in one place rather
  than inside the delivery module.

## Routing

- **Route** (`internal/platform/caddy`) — one fully-resolved public entrypoint the edge
  serves: the host it answers on, an optional path prefix, the upstream it proxies to, and its
  response headers. The owning module composes it; the platform module only translates and
  applies it. The routing analogue of `Content`. Every Route proxies — see **Edge** below.

- **RouteSource** (`internal/platform/caddy`) — the contribution seam. A module that owns
  public entrypoints implements `Routes(ctx)` and registers on the `Reconciler` at app
  wiring, mirroring `eventBusSvc.Register(auditSvc)`. Sources are **pulled** on every
  reconcile, never pushed: pulling is what keeps the desired configuration recomputable
  from stored state, which is the prerequisite for converging at startup and repairing
  drift at all.

- **Edge** (`deploy/caddy`) — the one Caddy that fronts every application on a host, owning its
  own listeners, ports, certificate storage and admin endpoint. It is the only thing that
  publishes a host port. It is *shared*, so it can read none of any one application's files:
  every Route proxies, and a built asset is served by an upstream beside the application.

- **Tenant** (`DINCHY_CADDY_TENANT`) — one application's identity on the edge. It namespaces
  the two configuration objects that application owns, so two deployments behind one edge
  address their own and never each other's.

- **Contribution** (`internal/platform/caddy`) — the slice of the edge's configuration this
  deployment owns, and the whole of what it writes: one `@id`-addressed route nesting every
  entrypoint as a subroute, plus one certificate automation policy naming their hosts. One
  route object rather than several is what makes the write atomic, makes a re-push idempotent
  across a restart, and prunes an entrypoint a source stopped reporting by leaving it out.

- **Reconciler** (`internal/platform/caddy`) — the apply seam. `ReconcileAll` applies the
  Contribution, once, at startup, addressing each object by its `@id` so every other tenant's
  routes survive. That is the only write there is: Dinchy does not re-assert on a timer,
  because the operator owns the running proxy once it is set up and other tenants own theirs.
  The policy goes first, because a route whose certificate could not be arranged fails its
  handshake while a certificate no route uses yet breaks nothing. The isolation is of
  configuration, not of connections — Caddy still reloads on any mutation.

- **Panel** — Dinchy's own UI and API, served as one `Route` like any deployment
  (`DINCHY_CADDY_PANEL_HOST`). It is the entrypoint that must not be lost: losing the panel
  means losing the only interface that could repair the routing. Reserving its host against
  other sources is deferred until there is a source that could claim it.

- **Upstream** — the `host:port` the edge dials for a `Route`. It is a different value from
  the address the upstream binds: a container listens on `0.0.0.0` and is reached by name, so
  the panel API advertises `DINCHY_CADDY_PANEL_UPSTREAM` and falls back to `DINCHY_ADDR` only
  when they coincide.

- The panel is *two* Routes on one hostname — `/api` proxying to Dinchy and the catch-all
  proxying to whatever serves the built assets (Vite in development, a static server beside
  the app otherwise). One hostname keeps the browser same-origin, and so keeps `SameSite=Lax`
  cookies and CSRF working without CORS. Within a host, a Route with a longer PathPrefix is
  matched first, because Caddy stops at the first terminal match.

- **Caddy build** (`cmd/caddy`) — vanilla upstream Caddy, existing only to pin the version
  through `go.mod`. It is what the edge image is built from, and what `caddy trust` runs from
  on a development machine. A plugin therefore belongs here as a blank import, pinned by
  `go.mod`; `xcaddy` against the same version (`task caddy:version`) stays available for a
  throwaway experiment. What a build actually provides is read with `caddy list-modules`,
  never from a manifest, because a Go module can resolve and build while registering no Caddy
  module.

- **TLS issuer** (`DINCHY_CADDY_TLS_ISSUER`) — `acme` for a public domain, `internal` for
  Caddy's own local CA. Dinchy never loads a certificate from disk; development uses the
  local CA because no public authority can validate localhost. mkcert remains only for
  Mailpit, which cannot generate its own.

## Transport

- **Renderer** (`internal/transport/render`) — the error-response rendering seam.
  It localizes a source-layer `errors.AppError` into the client-facing HTTP
  payload (`ResponsePayload`/`ErrorResponse`) for Huma, and owns whether internal
  failure detail is exposed. Rendering lives in transport, not in `internal/errors`:
  the errors package stays foundational and free of the HTTP framework.

- **Transport security is the edge's, not the app's.** Dinchy has no notion of whether a
  request arrived securely and no endpoint rejects one for being plaintext. The edge is the
  only intended ingress and serves only HTTPS. Two consequences to keep in mind when adding
  transport code: the request scheme must come from configuration (`Config.PublicScheme`, used
  for CORS origin matching), and the client address must come from `X-Forwarded-For` (read
  once in `RequestInfo`), because it is persisted to `audit_logs.ip_address`.

- **Trusted proxies** (`DINCHY_TRUSTED_PROXIES`) — what makes that forwarded address
  trustworthy. `DINCHY_ADDR` may face the edge's network rather than loopback, so anything else
  on that network could reach the listener directly; `RequestInfo` therefore honors
  `X-Forwarded-For` only from a peer inside the trusted set, and records the peer's own address
  otherwise. The default covers loopback alone, which is exactly what a host-native deployment
  behind the edge needs. A listener reachable beyond loopback with only loopback trusted is
  **rejected at startup**: it is not forgeable, but it would record the edge's address for every
  request, which is wrong in a way no operator would see.

- **Dinchy serves no documents, only the API.** Caddy delivers the web UI, so the
  Content-Security-Policy in `middleware.SecureHeaders` is a JSON policy —
  `default-src 'none'` — while the document policy naming `script-src`/`connect-src` belongs
  to whoever serves the HTML. A directive here that only a document could use is a sign of
  confusion about which response is being protected.

## Foundation

`internal/foundation/*` is the base tier: domain-agnostic primitives with zero
internal dependencies (or, for `errors`, only `i18n`). Nothing in `foundation`
imports `config`, `platform`, or any feature. It holds `errors`, `i18n`, `clock`,
`id`, `transform`, `requestmeta`, `security` (password hashing and tokens), and
`permission` (the access-control vocabulary). Everything else builds on top:
`platform/*` is infrastructure that depends on `config` and `foundation`.

- **requestmeta** (`internal/foundation/requestmeta`) — a foundation leaf
  carrying request-scoped observability values (client IP, user agent, and
  request/trace/span IDs) across layers via the context. Transport middleware
  populates it; any layer reads it, including non-transport code (e.g. the events
  envelope). It replaced the reach from `internal/platform/events` into
  `internal/transport/support`, so the event layer no longer depends on transport.
- **Password hashing** lives entirely in `internal/foundation/security` (algorithm
  *and* its Argon2id parameters/format constants); `security` is a foundation leaf
  that depends on nothing internal. **Config validation** is private to
  `internal/config`.
- **permission** (`internal/foundation/permission`) — the access-control
  vocabulary: the generated `Permission`/`Role` constants and their definitions.
  It is pure vocabulary (its only internal dependency is `i18n`), so it sits in
  `foundation` beside the other primitives rather than inside any feature. Source
  catalogs (`permissions.json`, `roles.json`) and codegen live alongside it.

## Features

- **Feature service base** (`internal/features`, `features.Service`) — the shared
  base every feature service embeds. It bundles the common infrastructure (clock,
  ID generator, database, Redis, cache keyer, mailer, event publisher, job
  enqueuer) and supplies a name-scoped contextual logger. `features.Service.Named`
  stamps a feature name; `Initialize` validates and defaults the dependencies.
  Feature packages (`internal/features/*`) embed `*features.Service`; the base
  imports only `platform/*` and `foundation/*`, never a feature, so a feature
  importing its parent package is cycle-free.
- **session** (`internal/features/session`) — the authenticated-request feature.
  It owns the `Principal` (the resolved user and active organization for a
  request), session token lifecycle (create, resolve, revoke, cache), the session
  cookie, and the request middleware that attaches the principal to the context.
  It is a standalone feature so transport middleware and `auth` can both depend on
  the principal seam without depending on each other.

## Events

- **Event bus** (`internal/platform/events`) — generic transport machinery only: the
  `Publisher`/`Subscriber` seam, Redis-stream delivery, consumer groups, the base
  types (`Type`, `Envelope`, `TypedEvent`, `Definition`, `Record`), and the shared
  `catalog.schema.json`. It imports no feature and owns no feature's vocabulary.
- **Feature-owned event definitions** — each feature declares its own events in a
  hand-written `events.json` (one top-level module named after the feature) and a
  generated `events_generated.go` (package = feature; stripped identifiers like
  `SecurityAuthLoginSucceeded`; a feature-scoped `EventDefinitions` map). Codegen
  discovers every `internal/features/*/events.json`, validates them as one catalog
  so types and names stay globally unique, and writes one generated file per feature.
- **Registration seam** — features register their `EventDefinitions` on the bus at
  app wiring (`eventBusSvc.RegisterDefinitions(auth.EventDefinitions)`), mirroring
  subscriber registration, so the bus can validate published events without
  importing feature code. Publishers live in the feature that emits the event; the
  audit feature is the sole subscriber.

## Ownership principle

Shared platform modules own the *machinery* (email delivery, event transport),
not the *feature composition*. Which email to send, its copy, and its links are
owned by the feature that sends it (e.g. auth composes invitation and password
reset emails in `auth_helpers.go`); the Mailer only delivers.
