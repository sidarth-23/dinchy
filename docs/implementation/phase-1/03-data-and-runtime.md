# Data And Runtime

## PostgreSQL Initialization

Phase 1 uses PostgreSQL via `pgx`.

Startup initialization path:

1. load startup config
2. open PostgreSQL
3. run embedded Goose migrations
4. ensure default singleton settings row exists
5. initialize stores, services, task runtime, and HTTP runtime

## Migrations

- Migrations are SQL files kept in-repo.
- They are embedded in the Go binary.
- Startup has a distinct migration step before serving.
- If migration fails, startup fails and the process exits.
- Tests must use the same embedded migration path.

## `sqlc` And Store Boundaries

- `sqlc` is used from Phase 1, configured via `sqlc.yaml` at the project root.
- PostgreSQL is the only concrete implementation.
- The `sqlc.yaml` has one `sql:` block for the Postgres store.

**Query organisation:**

```
internal/store/queries/    ← .sql files (PostgreSQL syntax — $1, RETURNING)
internal/store/sqlcgen/    ← sqlc-generated Go code (DO NOT EDIT)
internal/store/migrations/ ← goose migrations (PostgreSQL DDL — TIMESTAMPTZ, etc.)
```

**Consumer-defined interfaces (no monolithic Store):**

Each service declares the exact data access it needs:
- `internal/auth/store.go` — `auth.Store` interface
- `internal/workers/store.go` — `workers.Store` interface
- `internal/domain/settings.go` — `domain.SettingsReader` interface

The concrete `store.Store` implements all three. `internal/app/app.go` is the only place that imports the concrete store; it passes it to each service typed as that service's narrow interface.

**WithTx closure pattern:**

```go
err := s.WithTx(ctx, func(tx *Store) error {
    count, _ := tx.q.CountUsers(ctx)
    if count > 0 { return errors.New("setup already completed") }
    return tx.q.InsertUser(ctx, ...)
})
```

- `db == nil` means the Store is tx-scoped (backed by `*sql.Tx`).
- Calling `WithTx` on a tx-scoped Store is a passthrough — prevents accidental nested transactions.

**Wrapper translation:**

Generated sqlcgen structs are internal to the `store/` package. Wrapper methods translate between sqlcgen row types and the canonical domain types in `internal/domain/`. No code outside `store/` imports `sqlcgen`.

## IDs, Time, And Internal Types

- UUID is the primary key format for user/session/audit/task rows
- store UUIDs as canonical text in PostgreSQL
- generate IDs in application code through a shared `IDGenerator`
- the generator uses the shared `Clock`
- monotonic ULID generation should be used
- timestamps are UTC only

Internal duration handling:

- DB stores seconds
- services use `time.Duration`

## Schema

Phase 1 initial schema includes:

- `users`
- `sessions`
- `app_settings`
- `auth_audit_logs`
- `scheduled_tasks`

### `users`

Fields:

- `id` ULID primary key
- `email` lowercase canonical identifier, unique
- `password_hash`
- `display_name`
- `role`
- `disabled_at` nullable
- `last_login_at` nullable
- `created_at`
- `updated_at`

Notes:

- role is stored as validated slug text
- Go uses a string-backed `Role` type alias
- first user is always created as `admin`

### `sessions`

Fields:

- `id` ULID primary key
- `user_id`
- `token_hash`
- `ip_address`
- `user_agent`
- `last_seen_at`
- `idle_expires_at`
- `expires_at`
- `revoked_at` nullable
- `created_at`
- `updated_at`

Notes:

- `token_hash` is unique
- session validity is derived from `revoked_at`, `idle_expires_at`, and `expires_at`
- stale/expired sessions are invalid without needing write-on-read mutation

### `app_settings`

Singleton row.

Fixed ID:

- `id = 'app_settings'`

Fields:

- `instance_name`
- `session_idle_timeout_seconds`
- `session_max_lifetime_seconds`
- `session_cleanup_interval_seconds`
- `session_retention_seconds`
- `created_at`
- `updated_at`

Notes:

- defaults are seeded eagerly at startup if missing
- effective settings are cached in memory and refreshable immediately in-process

### `auth_audit_logs`

Fields:

- `id` ULID primary key
- `event_type`
- `user_id` nullable
- `actor_user_id` nullable
- `ip_address`
- `user_agent`
- `metadata_json`
- `created_at`

Notes:

- `event_type` values are namespaced
- table is auth-focused in Phase 1
- event metadata stores snapshots such as email and other event-time facts

Persisted auth events in Phase 1:

- `auth.setup_completed`
- `auth.login_succeeded`
- `auth.logout_succeeded`
- `auth.password_reset_by_admin` once reset exists
- `auth.login_rate_limited`
- `auth.setup_rate_limited`

### `scheduled_tasks`

Fields:

- `id` ULID primary key
- `task_name`
- `enabled`
- `schedule_interval_seconds`
- `lease_owner`
- `lease_expires_at`
- `last_run_at`
- `last_finished_at`
- `next_run_at`
- `last_status`
- `last_error_code`
- `last_error_message`
- `updated_at`

Notes:

- task rows are code-registered by name
- runtime state is persisted
- SQLite implementation is single-process now, but interfaces allow future distributed claims

## Settings And Policy Caching

Startup config, DB settings, and policy are intentionally separate.

Startup config:

- env and minimal flags for startup-critical values
- TOML config file for operator preferences

DB-backed mutable settings:

- instance name
- session idle/max lifetime
- cleanup interval
- session retention

Password hashing policy is startup config only in Phase 1.

## Durable Internal Task Runtime

Phase 1 includes a minimal persistent internal scheduler foundation.

Design rules:

- code-registered task handlers
- persisted schedule metadata and runtime state
- lease-based task claiming with expiration
- heartbeat lease renewal while a task runs
- per-task timeouts
- immediate wake/reschedule when schedule-affecting settings change
- no run history table in Phase 1

Phase 1 first task:

- `session_cleanup`

`session_cleanup` behavior:

- runs immediately once after startup in the background
- then follows its persisted interval
- deletes ended sessions older than retention in one SQL statement per run
- reports summary counts in logs and task result metadata

Task result rules:

- richer result object is kept
- task schedule does not change dynamically from result values in Phase 1
- timeout marks the run failed and reschedules normally
- repeated failures do not back off in Phase 1

## Readiness And Runtime Failure Rules

- server listens only after initialization succeeds
- `/readyz` should report not-ready if embedded frontend verification fails in production/default mode
- in `--dev` mode, embedded assets are not required and readiness may include `dev_proxy_reachable`
- request-level DB errors fail the request, not the whole process
- runtime-fatal component failures trigger coordinated shutdown

## Shutdown

- graceful shutdown is supported
- requests drain with a fixed timeout
- background task runtime is cancelled through shared app context
- non-critical task work should not delay shutdown completion
