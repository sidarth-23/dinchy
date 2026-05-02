# Scope And Deliverable

## Final Scope

Phase 1 is not just a server stub. It is the full foundation slice for the product:

- Go process lifecycle and package structure
- SQLite initialization, migrations, and initial schema
- Echo + Huma HTTP/API foundation
- Embedded frontend serving in production mode
- Explicit dev proxy mode
- First-user setup, login, logout, session validation
- CSRF and request security baseline
- Structured logs, request IDs, health/readiness endpoints
- Persistent scheduled-task foundation used by `session_cleanup`
- Frontend route skeleton and generated API client flow

## Explicitly Out Of Scope

These were intentionally deferred out of Phase 1:

- remote CLI/server auth flows
- broad settings UI
- OAuth/OIDC
- 2FA
- forgot password email flow
- Caddy integration
- container management
- user management beyond first admin setup and login/logout
- interactive role administration
- task run history UI

## Deliverable

The deliverable remains:

1. `go build` succeeds.
2. Start the binary.
3. Open the browser.
4. Land on first-user setup.
5. Create the initial admin.
6. Reach the dashboard.
7. Log out.
8. Log back in.
9. Reach the dashboard again.

## Backend Structure

Phase 1 should start with explicit package boundaries.

Recommended structure:

- `cmd/dinchy/`
- `internal/app/`
- `internal/config/`
- `internal/http/`
- `internal/http/middleware/`
- `internal/http/support/`
- `internal/api/`
- `internal/auth/`
- `internal/users/`
- `internal/settings/`
- `internal/tasks/`
- `internal/store/`
- `internal/store/sqlite/`
- `internal/frontend/`
- `internal/id/`
- `internal/clock/`
- `web/`

## Ownership Boundaries

- `internal/api` depends on application services only.
- `internal/api` (Huma layer) owns typed endpoint contracts and OpenAPI, not transport middleware orchestration.
- `internal/http` owns Echo server setup, middleware mounting, docs, health, and frontend route wiring.
- `internal/http` (Echo layer) owns transport concerns such as CORS, CSRF, request security headers, and Casbin middleware wiring.
- `internal/auth` owns password hashing, session issuance/validation/revocation, and auth-specific errors.
- `internal/users` owns user domain data and `viewer` projection.
- `internal/settings` owns DB-backed app settings, caching, and `app` projection.
- `internal/tasks` owns the durable internal scheduler and task runtime.
- `internal/store` defines interfaces and transaction boundaries.
- `internal/store/sqlite` contains the SQLite `sqlc` implementation.

## App Lifecycle

The app runtime should be explicit.

Recommended top-level API:

- `NewApp(...)`
- `Start()`
- `Shutdown(ctx)`
- `Wait() error`

Rules:

- `NewApp` wires dependencies but does not perform heavy startup work.
- `Start` opens the DB, applies pragmas, runs embedded migrations, seeds/loads settings, verifies frontend mode requirements, starts the task runtime, and begins listening.
- The process binds the port only after startup is ready.
- `Wait` returns the terminal runtime error when a critical component fails.

## Critical Runtime Rules

- If startup initialization fails, the process exits before serving.
- If frontend assets are invalid in production/default mode, startup fails.
- In `--dev` mode, embedded frontend assets are not required.
- HTTP server runtime failure is process-critical.
- Non-critical scheduled task run failures do not bring the app down.
