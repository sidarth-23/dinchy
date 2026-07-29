# Architecture Decisions

Key decisions made during design, with reasoning. This is the "why" behind the tech stack.

---

## Go binary over Docker container

Dinchy needs direct system access: spawning PTYs for web terminal, talking to the Docker socket natively, and reading `/proc/meminfo` for build safety. Running inside a Docker container would require mounting the host filesystem and using `nsenter` to escape into the host's namespace — complex and negating the isolation Docker provides. A Go binary runs directly on the host as a dedicated system user with exactly the permissions it needs (docker group, no sudo).

**Consequence:** Distribution is the Dinchy binary + a Caddy binary + service files. Installation is an install script, not `docker pull`.

The Caddy binary is deliberately **rebuildable by the operator** (`deploy/caddy/plugins.txt`, `mise run caddy:build`), which costs the "one artifact" story: compiling plugins needs a Go toolchain, git, and network access at install time. First-class plugin extensibility is worth more than a single-artifact install, and the default build is vanilla so only operators who want a plugin pay for it.

---

## PostgreSQL over SQLite/Redis

SQLite was originally considered for the platform's own data layer, but the implementation now standardizes on PostgreSQL. That keeps the runtime simple while avoiding a backend switch or multiple code paths.

PostgreSQL handles everything Dinchy needs:
- App state, deployment configs, user accounts — relational data
- Encrypted secrets — application-layer AES-256-GCM on sensitive columns
- Logs with full-text search — Postgres text search / indexed query patterns
- Container stats with rolling retention — simple time-series table with auto-prune
- Durable internal scheduled tasks and future job state — persisted in Postgres with lease-based recovery

At homelab scale (10-50 containers, tens of thousands of log lines/day), PostgreSQL remains the source of truth. Backups are handled with normal Postgres tooling.

---

## Caddy as sidecar (not embedded)

Caddy can be imported as a Go library (`github.com/caddyserver/caddy/v2`) and run inside the same binary. This was considered and rejected for three reasons:

1. **Failure isolation** — a Caddy bug or OOM from a misconfigured route should not crash the management plane. If routing breaks, the user needs the UI to remain accessible to fix it.
2. **Independent upgrades** — Caddy releases security patches frequently. Users should not need to wait for a Dinchy release to get a Caddy fix. The sidecar can be updated independently.
3. **Plugin flexibility** — homelab users want Caddy plugins (`caddy-cloudflare` for DNS-01 challenges behind NAT, `caddy-security` for auth middleware, etc.). With embedded Caddy, plugins are compiled in at build time. With a sidecar, users swap in their own `xcaddy` build.

The sidecar is controlled entirely via Caddy's [JSON Admin API](https://caddyserver.com/docs/api) at `localhost:2019`. Dinchy is the single source of truth — routes are stored in PostgreSQL and pushed to Caddy on change. No Docker labels, no config file templating, no process reloads.

### Full load at startup, one route at a time after

Dinchy replaces Caddy's **entire** configuration exactly once, at startup, and then addresses individual routes for every subsequent change.

The split exists because a full configuration reload makes Caddy close active streaming connections ([caddy#6420](https://github.com/caddyserver/caddy/issues/6420), [#7222](https://github.com/caddyserver/caddy/issues/7222)). The web terminal and live log viewer are WebSockets behind Caddy, so reloading on every routing change would drop one operator's terminal because another added a domain — and in development it would kill Vite HMR on every save. At startup there are no such connections, so converging in one call is free, and it is the only way to guarantee Caddy matches PostgreSQL after a restore or a restart.

Each route is tagged with a stable `@id` derived from its owner, host and path, which makes it addressable at `/id/<id>` without knowing its index in the route array.

Two invariants make the targeted path safe:

- **The configuration always contains an `admin` block.** `POST /load` replaces everything, and Caddy tears down the old admin endpoint on every load, so a document omitting it would destroy the very endpoint Dinchy pushes through — unrecoverable without editing files by hand.
- **Route match sets are kept disjoint.** An exact host that an existing wildcard would also match is rejected as a conflict, so appending a route can never change which route wins a request.

TLS is *not* a reload concern: Caddy keeps its certificate cache in a process-level global and reloads managed certificates from storage, so a reload with unchanged hostnames makes zero ACME requests. The limit that is actually reachable is failed authorizations per hour, which is why a domain whose DNS does not resolve is quarantined rather than retried indefinitely.

### Restart durability

Caddy runs as `caddy run --resume`, with no `--config`. The API is the source of truth, so Caddy's autosaved configuration *is* the state to restore. That is what makes restarting Caddy — after a plugin rebuild, or a crash — safe without Dinchy being up. `--watch` is irrelevant for the same reason: there is no configuration file to watch.

### Certificates must be served explicitly

Loading a certificate is not sufficient. For an SNI Caddy is not managing automatically, it needs a `tls_connection_policy` selecting that certificate by tag, or it aborts the handshake with a TLS internal error. Dinchy emits tagged certificates and matching policies, mirroring what Caddy's own Caddyfile adapter produces.

### Consequences

- Dinchy listens **plaintext on loopback** in every environment, development included. `DINCHY_TLS_*` no longer exists; there is exactly one TLS terminator and one place automatic HTTPS can live.
- Caddy owns HSTS, because the responses that most need it — the HTTP-to-HTTPS redirect and upstream-failure pages — are generated by Caddy and never reach Dinchy.
- **The app carries no transport check of its own.** It has no notion of whether a request was secure, and no endpoint rejects one for being plaintext. Caddy is the only ingress, it serves only HTTPS, and Dinchy binds loopback — so a plaintext request from the network is not something the app has to detect, it is something that cannot reach it.

  This replaced a per-request trusted-proxy check that could never fire: with a loopback listener, every peer that can connect *is* loopback, so the check always answered "trusted". It was also blind to the threat it was written for, since a host-networked container reaching the host's `127.0.0.1` looks identical to Caddy. The in-app 403 guards it fed were forgeable by any local process, so they never stopped a realistic attacker.

  The boundary is therefore enforced once, at startup: `config.Load` **rejects a non-loopback `DINCHY_ADDR`**. That check is load-bearing rather than advisory — an operator who removed it would expose the auth surface in plaintext.

- **The client IP comes from `X-Forwarded-For`** and is read in `RequestInfo`. This is a functional dependency, not a security one: the value is persisted to `audit_logs.ip_address` and `sessions.ip_address`, so reading the connection's own address instead would record loopback for every request. Caddy replaces the header rather than appending, so a client cannot forge it.

- **Caddy serves the web UI directly; Dinchy is API-only.** One hostname is split by path: `/api/*` reaches the Go process, everything else is the compiled SPA, which Caddy reads from disk in production and forwards to the Vite dev server in development. Asset requests never enter a Go goroutine, and — more importantly — the browser stays same-origin, so the session and CSRF cookies remain `SameSite=Lax` and CORS is never in play. A cross-origin split would have required `SameSite=None`, a different CSRF strategy, and an explicit `connect-src`.

  The consequence for **Content-Security-Policy** is easy to get wrong: CSP is a document policy, delivered on the HTML response. Since Dinchy no longer serves that document, its policy governs JSON only and is correspondingly strict — `default-src 'none'` — because a JSON response has no legitimate reason to load a script or a frame. The document policy naming `script-src`, `style-src` and `connect-src` belongs to whoever serves the HTML: Caddy in production, Vite in development. That is also why Dinchy has no development CSP variant any more; the relaxations hot reloading needs are Vite's to declare.

  Path ordering is correctness, not tidiness: Caddy stops at the first terminal match, so the `/api` route must precede the catch-all. `orderRoutes` sorts longest-prefix-first for that reason.

- **Caddy is the only place forwarded headers are normalized.** The generated configuration deletes `X-Real-IP`, `True-Client-IP` and `Forwarded`, which Caddy does **not** do by default — it passes them through untouched. Common Go middleware prefers those two over `X-Forwarded-For`, so the deletion is what stops a forged value from becoming the recorded client IP. It is covered by the contract test in `internal/platform/caddy` because nothing else would notice if it regressed.
- The proxied `Host` header is never rewritten: CORS origin checking and cookie scoping both depend on the original value.

---

## Caddy over Traefik/Nginx

**vs Traefik:** Traefik's Docker label discovery model makes Docker labels the source of truth for routing — competing with Dinchy's own state. Higher memory usage (50-200MB idle vs Caddy's ~30MB). Traefik has also benchmarked slower than Caddy in throughput tests.

**vs Nginx:** No API — routing changes require config file generation + `nginx -s reload`, which is a signal with no synchronous error channel, so a bad config surfaces in a log rather than to the caller that caused it. HTTPS requires certbot as a separate tool with its own cron job. Config syntax is a separate DSL to template correctly. Cannot be embedded in Go if needed later.

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
