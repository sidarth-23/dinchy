# Scope And Deliverable

## Final Scope

Phase 1 is not just a server stub. It is the full foundation slice for the product:

- Go process lifecycle and package structure
- SQLite initialization, migrations, and initial schema
- Chi + Huma HTTP/API foundation
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

Phase 1 uses a layered package layout grouped by concern. The top-level under `internal/` stays stable through all 8 phases — new features add packages inside each layer, not new top-level entries.

```
cmd/
  dinchy/
    main.go

internal/
  app/                          # Composition root — wires all dependencies
  config/                       # Startup config from environment variables

  domain/                       # Pure domain types — ZERO project imports
    users.go                    #   User, Role, CreateUserInput
    auth.go                     #   Session, SessionWithUser, CreateSessionInput
    settings.go                 #   BootstrapState, SettingsReader interface

  auth/                         # Auth business logic
    service.go                  #   Password hashing, session issuance/validation
    store.go                    #   Consumer-defined Store interface

  tasks/                        # Scheduled task runtime
    runtime.go                  #   Ticker loop, session_cleanup
    store.go                    #   Consumer-defined Store interface

  store/                        # Persistence layer
    store.go                    #   PostgreSQL implementation
      Open, Close, WithTx, migrations, store methods, sqlc adapter
      queries/                  #     sqlc query files (.sql) — PostgreSQL syntax
      sqlcgen/                  #     sqlc generated code (DO NOT EDIT)
      migrations/               #     goose migrations — PostgreSQL DDL

  server/                       # HTTP transport layer
    server.go                   #   Chi router setup, middleware stack, Huma mount, frontend serving
    internal.go                 #   Internal server (healthz, readyz)
    middleware/                 #   HTTP middleware (func(http.Handler) http.Handler)
      requestid.go, recover.go, realip.go, cleanpath.go, timeout.go  # Chi builtin wrappers
      cors.go                                                         # go-chi/cors wrapper
      https.go, requestinfo.go, lang.go, secure.go, csrf.go, session.go  # custom
    support/                    #   Shared transport helpers
      cookies.go, context.go
    api/                        #   Huma API handlers
      api.go, errors.go, bootstrap.go, setup.go, auth.go

  platform/                     # Shared infrastructure utilities
    clock/clock.go              #   Clock interface + RealClock
    id/id.go                    #   ULID generator
    frontend/frontend.go        #   embed.FS for compiled web UI

web/                            # Frontend source (Vite + React)
```

## Ownership Boundaries

- `internal/domain` owns all shared business types — zero project imports; the root of the dependency graph.
- `internal/auth` owns password hashing, session issuance/validation/revocation, and declares its own `Store` interface.
- `internal/tasks` owns the durable internal scheduler and declares its own `Store` interface.
- `internal/platform/store` is the only package that imports sqlc-generated code; it implements all consumer-defined store interfaces and translates between sqlcgen row types and domain types.
- `internal/server` owns the Chi router, all middleware, and all Huma API handlers. No router-specific types leak past the middleware layer into handlers.
- `internal/server/api` handlers receive `context.Context`, call service methods, and return domain errors. Transport concerns (cookies, IP, UA, language) are injected via `internal/server/support` context accessors.
- `internal/server/middleware` houses every middleware as its own file with a `func(http.Handler) http.Handler` signature. `server.go` only imports `mw.*` — swapping a middleware implementation touches one file.
- `internal/platform` owns standalone infrastructure utilities that have no domain knowledge.
- `internal/app` is the only package allowed to import the concrete store implementation (`store`); all other packages depend on interfaces.

## Multi-Database Seam

The store has a single Postgres-backed structure:
- `queries/*.sql` — database-specific SQL (placeholders, DDL, functions)
- `sqlcgen/` — sqlc-generated Go code for that engine
- `migrations/` — goose migration files in the Postgres DDL dialect

The `sqlc.yaml` at the project root has one `sql:` block. Nothing outside `store/` changes.

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
