# Testing And Workflow

## Testing Strategy

Phase 1 should validate the real foundation, not a mocked approximation.

Recommended layers:

- unit tests for pure helpers and validation logic
- backend integration tests with real disposable SQLite databases
- frontend route/startup tests for bootstrap branching
- one Playwright browser E2E golden path

## Backend Integration Defaults

- use a fresh app/DB per test by default
- create a disposable SQLite database per test
- use the same DB open/init path as production
- apply the same pragmas, including WAL mode
- run the same embedded migrations used by the app

Important integration coverage:

- first-user setup race-safe behavior
- session creation/validation/revocation
- bootstrap invariants
- cookie attributes
- trusted proxy secure detection
- client IP resolution from trusted proxies
- CSRF enforcement
- HTTPS-required policy

## Frontend Route Tests

Frontend tests should explicitly cover startup branching.

Minimum coverage:

- setup required routes to `/setup`
- anonymous routes to `/login`
- authenticated routes to dashboard/app shell
- `security.https_required` shows the HTTPS-required screen
- invalid bootstrap invariants show the startup error screen

## Browser E2E

Use Playwright.

Phase 1 suite expectations:

- one fresh app process per suite
- one fresh temp SQLite DB per suite
- run against the built embedded app, not dev proxy mode

Golden path:

1. visit app
2. see setup
3. create first admin
4. reach dashboard
5. logout
6. see login
7. log back in
8. reach dashboard again

Prefer UI behavior assertions over direct cookie inspection in E2E.

## Mise Tasks

All development tooling is managed by [mise](https://mise.jdx.dev/) via `mise.toml` at the project root. Install mise, then run `mise install` to fetch pinned versions of Go, Bun, Node, sqlc, goose, and golangci-lint.

Available tasks:

- `mise run dev` — run the Go backend in development mode
- `mise run build` — build the production binary (`./dinchy`)
- `mise run test` — run the full test stack (`go test ./...`)
- `mise run lint` — run golangci-lint
- `mise run fmt` — format Go code with golangci-lint
- `mise run generate` — regenerate generated Go code and sqlc queries (`go generate ./internal/errors ./internal/i18n && sqlc generate`)
- `mise run db:migrate` — run SQLite migrations (`goose up`)
- `mise run db:status` — check migration status
- `mise run web:dev` — run the frontend dev server (Bun runtime)
- `mise run web:build` — build production frontend assets
- `mise run web:install` — install frontend dependencies via Bun

Intent:

- `mise run dev` starts the backend in dev mode; the frontend dev server is started separately with `mise run web:dev`
- `mise run web:build` builds the frontend assets for embedding into the Go binary
- `mise run generate` regenerates the generated Go code and sqlc output
- `mise run build` produces the production binary
- `mise run test` runs the Phase 1 test stack

Bun is the primary JS runtime. Node is available via mise for compatibility, but all frontend tasks run through Bun.

## Verification Checklist

The implementation is only correct when all of the following are true:

- production/default mode fails fast if embedded frontend assets are missing or invalid
- `--dev` mode starts without embedded assets and uses the configured dev proxy URL
- setup is one-time and race-safe
- login/logout/session/bootstrap behavior matches the documented API contract
- cookies and CSRF behavior match the documented security policy
- trusted proxy handling affects secure cookies and client-IP resolution correctly
- durable `scheduled_tasks` runtime exists and `session_cleanup` runs on it
- docs/OpenAPI are public and same-origin API testing still obeys auth and CSRF rules
- health/readiness endpoints reflect the documented behavior
- backend integration tests, frontend route tests, and Playwright golden path all pass
