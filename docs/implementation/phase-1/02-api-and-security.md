# API And Security

## Transport Stack

- Echo is the outer HTTP server.
- Huma is mounted under `/api` via the Echo adapter.
- Huma owns typed request/response models, OpenAPI generation, and generated client compatibility.
- Echo remains confined to the transport layer. `echo.Context` must not leak into services.

Responsibility split:

- Huma owns API contract concerns:
  - operation registration and routing under `/api`
  - request decode/validation and typed response encode
  - OpenAPI generation and generated-client compatibility
- Echo owns transport and middleware concerns:
  - request ID and structured request logging
  - trusted proxy handling and secure-request detection
  - CORS policy, CSRF enforcement, same-origin checks, and security headers
  - auth/session middleware and Casbin authorization middleware
  - top-level non-OpenAPI routes such as `/healthz`, `/readyz`, and frontend serving

Guardrails:

- Huma handlers call service interfaces and return domain/application errors only.
- Echo middleware may translate transport concerns into structured API errors, but business logic does not depend on Echo types.

## API Shape

Phase 1 uses mixed REST/action-style endpoints.

Primary endpoints:

- `GET /api/bootstrap`
- `POST /api/setup/first-user`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/auth/session`

Operational/public endpoints:

- `GET /api/openapi.json`
- `GET /api/docs`
- `GET /healthz`
- `GET /readyz`

## JSON Rules

- API JSON uses `snake_case`.
- Requests and responses use `application/json` only.
- Unsupported request body media types return `415`.
- Malformed JSON returns `400`.
- Structured validation failures return `422`.

## Bootstrap Contract

`GET /api/bootstrap` is the startup state endpoint.

Normal states always return `200`.

Response shape:

```json
{
  "setup_required": false,
  "authenticated": true,
  "app": {
    "instance_name": "Dinchy"
  },
  "viewer": {
    "email": "admin@example.com",
    "display_name": "Admin",
    "role": "admin"
  }
}
```

Invariants:

- if `setup_required = true`, then `authenticated = false` and `viewer = null`
- if `authenticated = true`, then `setup_required = false` and `viewer != null`
- invalid combinations are contract violations and should be treated as startup failure on the frontend

`bootstrap` also:

- ensures the CSRF cookie exists
- clears stale/invalid session cookies
- applies HTTPS-required policy when configured

## Session Model

- Session cookie name: `dinchy_session`
- Cookie value: opaque random token, base64url without padding
- Server stores: SHA-256 of raw token bytes
- Multiple concurrent sessions are allowed across browsers/devices
- Re-login in the same browser replaces the current browser session

Session row semantics:

- `idle_expires_at` and `expires_at` are persisted on creation/update
- existing sessions keep their stored expiry semantics even if session policy changes later
- invalid sessions are treated as anonymous on read; no write-on-read is required

## Cookie Policy

Session cookie:

- `HttpOnly = true`
- `SameSite = Lax`
- `Path = /`
- host-only cookie, no `Domain`
- `Secure` depends on trusted per-request secure detection

CSRF cookie:

- name: `dinchy_csrf`
- not `HttpOnly`
- `SameSite = Lax`
- `Path = /`
- host-only cookie, no `Domain`
- `Secure` depends on trusted per-request secure detection

## CSRF Model

Phase 1 uses the double-submit cookie pattern.

- CSRF token is a pure random secret.
- Frontend and docs UI read the `dinchy_csrf` cookie and send it in `X-CSRF-Token`.
- CSRF is enforced on mutating API methods:
  - `POST`
  - `PUT`
  - `PATCH`
  - `DELETE`
- `GET`, `HEAD`, and `OPTIONS` are exempt.
- CSRF token rotates on:
  - successful setup
  - successful login
  - logout

Logout replaces the CSRF cookie with a fresh anonymous token.

## HTTPS And Origin Policy

Trusted proxy support exists in Phase 1.

- trust forwarded headers only from configured trusted proxies
- support both `Forwarded` and `X-Forwarded-*`
- secure request detection uses direct TLS or trusted forwarded proto

Optional startup policy:

- `require_https_for_auth`

When enabled, these endpoints reject insecure requests with `403 security.https_required`:

- `GET /api/bootstrap`
- `GET /api/auth/session`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `POST /api/setup/first-user`

Frontend behavior:

- the shell may still load
- a dedicated HTTPS-required screen is shown
- if configured `public_url` is HTTPS, show a best-effort secure link

Mutating requests also enforce same-origin policy:

- if `Origin` is present, it must match the configured/trusted request origin

## Error Contract

Errors use one structured envelope.

```json
{
  "error": {
    "code": "auth.invalid_credentials",
    "message": "Invalid email or password.",
    "request_id": "01J..."
  }
}
```

Validation failures add field details:

```json
{
  "error": {
    "code": "request.validation_failed",
    "message": "Some fields need attention.",
    "request_id": "01J..."
  },
  "fields": {
    "email": {
      "message": "Enter a valid email address.",
      "rule": "email"
    }
  }
}
```

Rules:

- error codes are namespaced
- client and server i18n stay independent
- frontend maps exact codes first, then namespace fallbacks, then server message
- request IDs appear in response headers for all requests and in error bodies for failures

Important status mapping:

- malformed JSON: `400`
- validation failure: `422`
- invalid credentials: `401`
- unauthenticated protected API access: `401`
- CSRF/origin failure: `403`
- HTTPS-required auth failure: `403`
- setup already completed: `409`
- rate limited: `429`
- not ready: `503`
- unknown internal failure: `500`

## Public Docs

- OpenAPI JSON and docs UI are public.
- They live under `/api`.
- Docs UI uses the same-origin browser session/cookie model naturally.
- Docs UI is not exempt from auth, CSRF, or origin rules for actual API calls.

## Health And Readiness

- `/healthz` is top-level plain text and liveness-oriented.
- `/readyz` is top-level JSON and readiness-oriented.
- They are not part of OpenAPI.

Recommended `/readyz` shape:

```json
{
  "ready": true,
  "version": "dev",
  "checks": {
    "database": true,
    "migrations": true,
    "frontend_embed": true
  }
}
```

In `--dev` mode, include `dev_proxy_reachable` as additional diagnostic information.

## Browser Security Headers

Phase 1 should ship with an app-layer baseline:

- `Content-Security-Policy`
- `X-Content-Type-Options: nosniff`
- `Referrer-Policy: no-referrer`
- `X-Frame-Options: DENY`
- `frame-ancestors 'none'`

Notes:

- use a strict default CSP in production/default mode
- use a relaxed dev CSP in `--dev`
- allow minimal route-specific CSP exceptions for `/api/docs` if required
- only send HSTS on secure requests
