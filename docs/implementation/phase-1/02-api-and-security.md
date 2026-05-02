# API And Security

## Transport Stack

- Chi is the outer HTTP router (`internal/server/server.go`).
- Huma is mounted under `/api` via the Chi adapter (`humachi.New`).
- Huma owns typed request/response models, OpenAPI generation, and generated client compatibility.
- No router-specific types must leak past the middleware layer into handlers or services.

Responsibility split:

- **Huma** owns API contract concerns:
  - operation registration via `huma.Register(h, huma.Operation{...}, handler)`
  - typed request decode/validation and response encode
  - OpenAPI generation and generated-client compatibility
  - all handler signatures use `func(ctx context.Context, in *I) (*O, error)`
- **Chi middleware** owns transport concerns:
  - request ID and panic recovery
  - real IP resolution from proxy headers
  - HTTPS/secure-request detection (`internal/server/middleware/https.go`)
  - client IP and User-Agent extraction (`internal/server/middleware/requestinfo.go`)
  - security headers CSP, X-Frame-Options, Referrer-Policy (`internal/server/middleware/secure.go`)
  - CORS policy and CSRF double-submit cookie enforcement (`internal/server/middleware/csrf.go`)
  - session cookie validation and context injection (`internal/server/middleware/session.go`)
  - top-level non-OpenAPI routes: `/healthz`, `/readyz`, `/api/docs` redirect, frontend serving

All middleware in `internal/server/middleware/` uses the standard `func(http.Handler) http.Handler`
signature — router-agnostic and HTTP-compliant. Each middleware has its own file as a single source of truth.

Middleware stack order in `server.New()`:
1. `mw.RequestID()` — injects unique request ID
2. `mw.Recover()` — panic recovery → 500
3. `mw.RealIP()` — resolves `RemoteAddr` from `X-Real-IP` / `X-Forwarded-For`
4. `mw.CleanPath()` — normalises double slashes
5. `mw.SecureDetect()` — injects `IsSecure` into context
6. `mw.RequestInfo()` — injects `RemoteIP` + `UserAgent` into context
7. `mw.Lang(catalog)` — injects resolved `language.Tag` into context
8. `mw.SecureHeaders(devMode)` — CSP, X-Frame-Options, Referrer-Policy
9. `mw.CORS()` — CORS via `go-chi/cors`
10. `mw.CSRF()` — double-submit cookie, skip safe methods
11. `mw.Session(authSvc)` — cookie → `*domain.SessionWithUser` in context
12. `mw.Timeout(30s)` — request deadline

## Context-Value Bridge

Huma handlers receive `context.Context`. Middleware injects request-scoped values into context via `internal/server/support`:

- `support.WithSession(ctx, sess)` → `support.SessionFrom(ctx)`
- `support.WithSecure(ctx, bool)` → `support.IsSecure(ctx)`
- `support.WithRequestInfo(ctx, ip, ua)` → `support.RemoteIPFrom(ctx)` / `support.UserAgentFrom(ctx)`
- `support.WithLang(ctx, tag)` → `support.LangFrom(ctx)`

Cookie setting uses huma's native `header:"Set-Cookie"` output struct field — no router context needed in handlers.

Guardrails:

- Huma handlers call service interfaces and return `*DinchyError` values (implementing `huma.StatusError`).
- Business logic does not depend on Chi or any router types at all.
- `huma.NewError` is overridden at server startup so `*apierr.LocalizedError` is serialised as `{"code":"...","message":"..."}` rather than wrapped in Huma's default error envelope.

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

Errors are returned as `*DinchyError` values defined in `internal/server/api/errors.go`. They implement `huma.StatusError` so huma serialises them with the correct HTTP status code.

```json
{
  "code": "auth.invalid_credentials",
  "message": "Invalid email or password."
}
```

Predefined constructors: `ErrInvalidCredentials()`, `ErrSetupCompleted()`, `ErrUnauthenticated()`, `ErrHTTPSRequired()`, `ErrInternal()`. Domain errors from services are mapped via `MapServiceError(err)`.

The full documented envelope (with `request_id` and `fields`) is the target shape; the `request_id` field is added when `huma.NewError` is overridden globally. Huma's built-in `422` validation errors produce field details automatically.

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
