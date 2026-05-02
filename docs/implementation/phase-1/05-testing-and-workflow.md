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

## OpenAPI And Client Generation

OpenAPI generation should not require running the server.

Recommended pattern:

- a dedicated Go command builds the Huma API and writes the spec
- Taskfile runs spec generation and frontend client generation

## Taskfile Expectations

Planned tasks:

- `task dev`
- `task web:build`
- `task api:generate`
- `task build`
- `task test`

Intent:

- `task dev` runs backend plus frontend dev server flow
- `task web:build` builds the frontend assets
- `task api:generate` regenerates the OpenAPI-based frontend client
- `task build` produces the production binary
- `task test` runs the Phase 1 test stack

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
