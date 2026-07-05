# Dinchy — Implementation Plan

8 phases, each with a concrete deliverable. Build one phase at a time.

---

## Phase 1: Foundation

**Goal:** Runnable skeleton - Go backend serves a React frontend, Postgres is set up, basic auth works.

**Backend:**
- Layered Go project structure (`cmd/dinchy/`, `internal/{app,config,domain,auth,tasks,store,server,platform}`)
- Chi + Huma API with typed operations, OpenAPI metadata, input validation, and structured error responses
- PostgreSQL-backed store (`pgx`, `pressly/goose`), sqlc-generated queries (`sqlc.yaml`)
- Database migrations (`pressly/goose`) for the single Postgres store implementation
- Consumer-defined store interfaces per domain (`auth.Store`, `workers.Store`, `domain.SettingsReader`)
- Initial schema: users, sessions, app_settings, app_audit_logs, scheduled_tasks
- Chi middleware stack: request ID, real IP, CORS, CSRF double-submit, session validation, security headers, HTTPS detection
- `embed.FS` serving the frontend build
- Persistent internal scheduled task foundation for `session_cleanup`

**Frontend:**
- Vite + React + TypeScript scaffold
- TanStack Router with explicit startup branching and protected app layout
- Shadcn/ui base layout (sidebar, header, content area)
- Login page, first-user signup page
- Generated API client from OpenAPI

**Auth:**
- Email/password login (argon2 hashing)
- First-user signup (first visitor creates admin, registration locks)
- Session management (HTTP-only cookies in SQLite)
- CSRF protection, request security middleware, and auth audit logging

**Deliverable:** `go build` → start binary → open browser → create admin → log in → empty dashboard.

**Detailed implementation docs:** See [`docs/implementation/phase-1/`](implementation/phase-1/README.md).

---

## Phase 2: Container Management Core

**Goal:** Deploy and manage containers through the UI. The core value loop works.

**Interfaces (define all four, implement Docker versions):**
- `ContainerRuntime` → `DockerRuntime` (Docker socket via `github.com/docker/docker/client`)
- `HostExecutor` → `LocalExecutor`
- `SecretStore` → `SQLiteSecretStore` (AES-256-GCM encrypted)
- `BuildEngine` interface only (implement in Phase 4)

**Container Image mode:**
- UI form: image, ports, env vars, volumes, restart policy
- Backend: pull image, create + start container, track state in SQLite
- Container lifecycle: start, stop, restart, remove

**Registry support:**
- Settings page: add/edit/delete registry credentials (URL, username, token)
- Encrypted via `SecretStore`
- Registry picker when deploying private images

**Env vars / secrets:**
- Key-value editor in deployment UI, values masked (click to reveal)
- Stored encrypted, redacted from logs

**Schema additions:** projects, deployments, containers, registries, secrets tables

**Deliverable:** Deploy `nginx:latest` from UI, see it running, stop/restart, configure env vars.

---

## Phase 3: Caddy Integration & Routing

**Goal:** Deployed containers are reachable via domain names with automatic HTTPS.

**Caddy sidecar:**
- Go HTTP client wrapping Caddy Admin API (`localhost:2019`)
- Route add/update/remove on deployment change

**Domain routing UI:**
- Per-deployment domain assignment
- Port mapping
- Auto-HTTPS toggle (Let's Encrypt for public, self-signed for local)

**Customization:**
- Custom headers, CORS, redirects via UI → Caddy API
- "Advanced config" textarea for raw Caddy JSON

**Deliverable:** Deploy nginx, assign `test.home.lab`, access via browser over HTTPS.

---

## Phase 4: Docker Compose & Git Deployments

**Goal:** All three deployment modes work, with build safety.

**Docker Compose mode:**
- Paste/upload compose file, preview services before deploy
- Lifecycle via `github.com/docker/compose/v2`
- Per-service domain routing

**Git + Dockerfile mode:**
- Input: repo URL, branch, Dockerfile path, build args
- Clone → build → tag → deploy
- Implement `DockerfileBuild` for `BuildEngine` interface

**Build Resource Guardian:**
- Pre-flight: read `/proc/meminfo`, refuse if below threshold
- Queue: channel-based, one build at a time on <16GB (configurable)
- Watchdog goroutine: poll `/proc/meminfo` every 2s, graceful abort on critical
- cgroup ceiling: 50% of available RAM
- `oom_score_adj = -1000` on Dinchy process
- UI: build memory timeline, failure suggestions

**Deliverable:** Multi-service compose deploy works. Astro app builds from Git without OOM crash.

---

## Phase 5: Terminal & Log Viewer

**Goal:** Full visibility into running containers.

**Web terminal:**
- `creack/pty` spawns PTY as `dinchy` system user
- WebSocket transport → xterm.js frontend
- Command audit log to SQLite

**Real-time log streaming:**
- Docker API `ContainerLogs` + `Follow: true` → WebSocket → frontend

**Log viewer UI (separate from terminal):**
- Virtualized scrolling (`@tanstack/react-virtual`)
- Syntax highlighting by level (ERROR=red, WARN=yellow, INFO=blue)
- Search/filter bar, auto-scroll, timestamp toggle, JSON pretty-print

**Historical log search:**
- Persist logs to SQLite FTS5
- Configurable retention (default 24h)
- API: keyword + container + level + time range

**Deliverable:** Web terminal works. Live log stream works. Historical search works.

---

## Phase 6: Full Auth & Authorization

**Goal:** Multi-user support with SSO and RBAC.

**OAuth/OIDC (`markbates/goth`):**
- Providers: Google, GitHub, generic OIDC
- Settings: issuer URL, client ID, secret
- "Login with..." on login page

**2FA (`pquerna/otp`):**
- TOTP enrollment (QR code in user settings)
- 2FA gate on login
- Recovery codes

**Forgot password:**
- Email reset (requires SMTP config)
- Time-limited single-use tokens

**RBAC (`apache/casbin`):**
- Admin: full access
- Deployer: deploy + logs + env vars, no user management
- Viewer: read-only
- Role mapping from OIDC `groups` claim

**User management:**
- Invite via email or shareable link
- List/edit/disable/delete
- Role assignment

**Deliverable:** Teammate logs in via Google OAuth as Viewer. Admin enables 2FA.

---

## Phase 7: Monitoring, Alerts & Rollback

**Goal:** 80% of users don't need a separate monitoring stack.

**Container stats:**
- Poll Docker stats API every 10s
- SQLite with 24h rolling retention
- CPU/memory sparklines on dashboard

**HTTP health checks:**
- Per-deployment URL + interval (30-60s)
- Response time + status code history
- Uptime % graph

**Deploy history + rollback:**
- Every deploy records previous state (image, compose, env vars) in SQLite
- Deploy timeline in UI
- One-click rollback

**Alerts (webhooks only):**
- POST on: container crash, health check fail, disk >90%, memory >90%
- Single webhook URL in settings — works with ntfy, Slack, Discord, everything

**Deliverable:** Sparklines visible. Health check uptime graph works. Rollback works. Webhook fires on crash.

---

## Phase 8: Distribution & Polish

**Goal:** Clean install, auto-updates, production-ready.

**Install script:**
- Creates `dinchy` system user (docker group, no sudo)
- Downloads main + updater binaries (verified checksum)
- Detects systemd vs OpenRC, installs service files
- Enables and starts services

**Service files:**
- `dinchy.service` (systemd) + OpenRC init script
- `dinchy-update.timer` (systemd) + cron (OpenRC)
- `OOMScoreAdjust=-1000` in systemd unit

**Updater binary:**
- Checks GitHub releases API
- Download → verify checksum + signature → keep `dinchy.old` → restart
- Health check post-restart; auto-rollback to `dinchy.old` if fails

**Polish:**
- Error messages, loading states, empty states
- Dark mode (Shadcn native)
- Responsive layout
- Settings: general (port, hostname), SMTP, update channel, log retention
- System info page: version, memory, disk, uptime

**Deliverable:** `curl -fsSL https://dinchy.com/install.sh | bash` → running. UI update → restarts cleanly.

---

## Future (v2+)

| Feature | Notes |
|---|---|
| Multi-node | gRPC master/agent; `HostExecutor` → `RemoteExecutor` |
| Plugin ecosystem | Container-based plugins; DuckDB log analytics as first plugin |
| App template store | Community-contributed compose templates |
| Podman support | `ContainerRuntime` → `PodmanRuntime` |
| Buildpacks / Nixpacks | `BuildEngine` → `BuildpackBuild`, `NixpackBuild` |
| External secrets | `SecretStore` → `VaultSecretStore`, `SOPSSecretStore` |
| Scheduled jobs | Cron-based container runs |
| Backup/restore | Automated SQLite + volume backup |
| CLI | `dinchy` CLI for managing deployments without browser |
