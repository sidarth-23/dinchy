# Phase 1 Implementation

This directory captures the final implementation decisions for Phase 1 after the detailed planning discussion.

Phase 1 goal:
- `go build`
- start the binary
- open the browser
- create the first admin
- land on the dashboard
- log out
- log back in
- land on the dashboard again

This is the source of truth for how Phase 1 should be built.

## Document Map

- [`01-scope-and-deliverable.md`](01-scope-and-deliverable.md)
  - final Phase 1 scope, exclusions, deliverable, and package layout
- [`02-api-and-security.md`](02-api-and-security.md)
  - HTTP stack, API contract, cookies, CSRF, HTTPS policy, error model, docs UI, health endpoints
- [`03-data-and-runtime.md`](03-data-and-runtime.md)
  - PostgreSQL, migrations, schema, settings, durable scheduled tasks, lifecycle, shutdown
- [`04-frontend-and-ux.md`](04-frontend-and-ux.md)
  - router structure, startup branching, pages, form strategy, generated client usage
- [`05-testing-and-workflow.md`](05-testing-and-workflow.md)
  - integration/E2E defaults, OpenAPI generation, mise workflow, verification expectations

## Core Locked Decisions

- Production is a single Go binary serving embedded frontend assets.
- Development uses `DINCHY_DEV=1` mode; embedded assets are not required in this mode.
- Chi is the outer HTTP router and middleware host. Huma owns the typed API contract and OpenAPI generation under `/api` via the `humachi` adapter. Handler signatures are `func(context.Context, *I) (*O, error)` — no router-specific context leaks into handlers. All middleware uses standard `func(http.Handler) http.Handler`.
- PostgreSQL is the only Phase 1 database implementation. sqlc generates queries from `internal/platform/store/queries/*.sql`. The `internal/platform/store` package owns the concrete Postgres store.
- Consumer-defined store interfaces: `auth.Store`, `workers.Store`, `domain.SettingsReader`. `internal/app/app.go` imports the concrete `store.Store`.
- Internal packages are grouped by layer: `domain/` (pure types), `auth/` + `tasks/` (business logic), `store/` (persistence), `server/` (HTTP), `platform/` (utilities). The top-level under `internal/` stays stable through all 8 phases.
- First-user setup is race-safe (transactional count check) and closes permanently once the first admin exists.
- Auth uses opaque session cookies with SHA-256 token hashing, DB-backed sessions, CSRF double-submit cookies, and structured security middleware.
- Phase 1 includes a small persistent internal scheduled-task foundation, and `session_cleanup` runs on it immediately.
- Route and bootstrap behavior are explicit and tested, not inferred through ad hoc UI state.

## Existing Documents Updated

- [`../../plan.md`](../../plan.md)
  - Phase 1 summary updated to match the final design
- [`../../architecture.md`](../../architecture.md)
  - background task persistence and Phase 1 HTTP stack clarified
