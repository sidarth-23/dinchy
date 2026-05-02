# Architecture Decisions

Key decisions made during design, with reasoning. This is the "why" behind the tech stack.

---

## Go binary over Docker container

Dinchy needs direct system access: spawning PTYs for web terminal, talking to the Docker socket natively, and reading `/proc/meminfo` for build safety. Running inside a Docker container would require mounting the host filesystem and using `nsenter` to escape into the host's namespace — complex and negating the isolation Docker provides. A Go binary runs directly on the host as a dedicated system user with exactly the permissions it needs (docker group, no sudo).

**Consequence:** Distribution is a single binary + Caddy binary + service files. Installation is an install script, not `docker pull`.

---

## SQLite over PostgreSQL/Redis

Dokploy uses PostgreSQL + Redis. PostgreSQL idles at 50-200MB. Redis adds another 10-50MB. That's 60-250MB of memory consumed before your app even starts — just for the platform's own data layer.

SQLite with WAL mode handles everything Dinchy needs:
- App state, deployment configs, user accounts — relational data
- Encrypted secrets — application-layer AES-256-GCM on sensitive columns
- Logs with full-text search — SQLite FTS5 extension
- Container stats with rolling retention — simple time-series table with auto-prune
- Durable internal scheduled tasks and future job state — persisted in SQLite with lease-based recovery

At homelab scale (10-50 containers, tens of thousands of log lines/day), SQLite is never the bottleneck. The single-file database means backups are `cp dinchy.db dinchy.db.backup`.

**Driver:** `modernc.org/sqlite` — pure Go, no CGO. Keeps cross-compilation clean (no need for platform-specific CGO toolchains).

---

## Caddy as sidecar (not embedded)

Caddy can be imported as a Go library (`github.com/caddyserver/caddy/v2`) and run inside the same binary. This was considered and rejected for three reasons:

1. **Failure isolation** — a Caddy bug or OOM from a misconfigured route should not crash the management plane. If routing breaks, the user needs the UI to remain accessible to fix it.
2. **Independent upgrades** — Caddy releases security patches frequently. Users should not need to wait for a Dinchy release to get a Caddy fix. The sidecar can be updated independently.
3. **Plugin flexibility** — homelab users want Caddy plugins (`caddy-cloudflare` for DNS-01 challenges behind NAT, `caddy-security` for auth middleware, etc.). With embedded Caddy, plugins are compiled in at build time. With a sidecar, users swap in their own `xcaddy` build.

The sidecar is controlled entirely via Caddy's [JSON Admin API](https://caddyserver.com/docs/api) at `localhost:2019`. Dinchy is the single source of truth — routes are stored in SQLite and pushed to Caddy on change. No Docker labels, no config file templating, no process reloads.

---

## Caddy over Traefik/Nginx

**vs Traefik:** Traefik's Docker label discovery model makes Docker labels the source of truth for routing — competing with Dinchy's own state. Higher memory usage (50-200MB idle vs Caddy's ~30MB). Traefik has also benchmarked slower than Caddy in throughput tests.

**vs Nginx:** No API — routing changes require config file generation + `nginx -s reload`. HTTPS requires certbot as a separate tool with its own cron job. Config syntax is a separate DSL to template correctly. Cannot be embedded in Go if needed later.

**Caddy wins** because: JSON Admin API allows atomic route changes from Go code with zero downtime; automatic HTTPS with no certbot; ~30MB idle memory; written in Go.

---

## systemd + OpenRC (not Docker for supervision)

Using Docker to run Dinchy itself was considered. Rejected because:
- Web terminal via PTY requires the process to be on the host, not inside a container
- Reading host system metrics (`/proc/meminfo`, disk usage) is simpler from the host
- Docker-in-Docker for managing user containers adds a layer of indirection

systemd covers 95%+ of homelab host OSes (Debian, Ubuntu, Fedora, Arch, Proxmox, RHEL). OpenRC covers Alpine and Gentoo. The binary is init-system-agnostic — it just runs in the foreground. Service files are packaging artifacts, not architecture.

`OOMScoreAdjust=-1000` in the systemd unit prevents the Linux OOM killer from ever targeting the management plane, even during a runaway build.

---

## Separate updater binary

The updater is a small, intentionally-dumb Go binary that almost never changes. It:
1. Checks GitHub releases API for new versions
2. Downloads + verifies the new binary (checksum + signature)
3. Keeps the old binary as `dinchy.old`
4. Restarts the main service via systemd/OpenRC
5. Health-checks the new version; auto-rolls back if it fails to start within 10s

Keeping update logic in a separate binary means: if the update mechanism itself has a bug, users can run the updater manually to get a fixed version. If the update logic were inside the main binary, a broken update mechanism would require manual intervention to fix.

---

## Build-it-yourself auth (not Casdoor)

[Casdoor](https://casdoor.org/) was evaluated — it's by the same team as Casbin, written in Go, Docker image is 63.5MB, runtime is ~100MB. It provides email/password, OAuth, 2FA, and user management out of the box.

Rejected because: 100MB is 50% of the entire Dinchy memory budget just for auth. Casdoor runs as a separate process with its own HTTP server and React frontend, creating a disjointed UX. The login page and user management would look like Casdoor, not Dinchy. Building auth in-house with Goth + Casbin + pquerna/otp costs 3-4 weeks of development but produces a seamlessly integrated experience within Dinchy's Shadcn UI.

**Auth stack:**
- `markbates/goth` — OAuth/OIDC for 30+ providers
- `apache/casbin` — RBAC policy enforcement
- `pquerna/otp` — TOTP for 2FA
- `golang.org/x/crypto/argon2` — password hashing

**Phase 1 transport stack:**
- `go-chi/chi/v5` — outer HTTP router and middleware host
- `go-chi/cors` — CORS middleware
- `github.com/danielgtaylor/huma/v2` — typed API contract, OpenAPI generation, generated client workflow (mounted via `humachi` adapter)

---

## Key extensibility interfaces

Four interfaces are defined from the start to avoid rewriting core logic when extending:

```go
// DockerRuntime → PodmanRuntime
type ContainerRuntime interface { ... }

// LocalExecutor → RemoteExecutor (gRPC for multi-node)
type HostExecutor interface { ... }

// SQLiteSecretStore → VaultSecretStore, SOPSSecretStore
type SecretStore interface { ... }

// DockerfileBuild → BuildpackBuild, NixpackBuild
type BuildEngine interface { ... }
```

v1 ships one implementation of each. v2+ adds implementations without changing any consuming code.

---

## Build Resource Guardian

The specific pain point: deploying an Astro app via `docker build` on an 8GB VPS crashes the server. Root cause: BuildKit's `--memory` flag is [not reliably enforced](https://github.com/moby/buildkit/issues/1362), and the Linux OOM killer fires indiscriminately.

Dinchy's solution operates at four layers:
1. **Pre-flight** — refuse to start a build if available memory is below threshold
2. **Queue** — serialize builds (one at a time on <16GB machines)
3. **Watchdog** — goroutine monitors `/proc/meminfo` during builds, gracefully aborts if critical
4. **Protection** — `oom_score_adj = -1000` ensures Dinchy itself is never OOM-killed

Result: users see "Build aborted: insufficient memory" in the UI rather than a dead server.

The Phase 1 implementation also introduces a small persistent scheduled-task foundation in SQLite so internal workers survive restart and do not rely on ephemeral in-memory loops alone.
