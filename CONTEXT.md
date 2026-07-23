# Dinchy domain context

Ubiquitous language for the codebase. Names here are the ones to use in code,
tests, and design discussion. Keep entries terse and intent-focused.

This file is domain facts, not rules of conduct: behavioral rules live in
`.rules` and their task-specific detail in `.agents/skills/*`.

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

## Transport

- **Renderer** (`internal/transport/render`) — the error-response rendering seam.
  It localizes a source-layer `errors.AppError` into the client-facing HTTP
  payload (`ResponsePayload`/`ErrorResponse`) for Huma, and owns whether internal
  failure detail is exposed. Rendering lives in transport, not in `internal/errors`:
  the errors package stays foundational and free of the HTTP framework.

## Foundation

`internal/foundation/*` is the base tier: domain-agnostic primitives with zero
internal dependencies (or, for `errors`, only `i18n`). Nothing in `foundation`
imports `config`, `platform`, or any feature. It holds `errors`, `i18n`, `clock`,
`id`, `transform`, `requestcontext`, `security` (password hashing and tokens), and
`permission` (the access-control vocabulary). Everything else builds on top:
`platform/*` is infrastructure that depends on `config` and `foundation`.

- **requestcontext** (`internal/foundation/requestcontext`) — a foundation leaf
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
