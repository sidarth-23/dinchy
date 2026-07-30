# Architecture Decisions

Key decisions made during design, with reasoning. This is the "why" behind the tech stack.

---

## Go binary over Docker container

> **Superseded.** Dinchy runs as a container. What survives is the *reasoning* below about what a
> container must therefore be granted — and the grants are explicit rather than incidental. See
> "Containers over host binaries" immediately after this section.

Dinchy needs direct system access: spawning PTYs for web terminal, talking to the container socket natively, and reading `/proc/meminfo` for build safety. Running inside a Docker container would require mounting the host filesystem and using `nsenter` to escape into the host's namespace — complex and negating the isolation Docker provides. A Go binary runs directly on the host as a dedicated system user with exactly the permissions it needs (docker group, no sudo).

The Caddy binary is deliberately **rebuildable by the operator**, which costs the "one artifact" story: compiling it needs a Go toolchain and network access at install time. First-class plugin extensibility is worth more than a single-artifact install. `cmd/caddy` is a vanilla build whose only job is to pin the Caddy version through `go.mod`. That part is unchanged; only where the build lands has moved.

---

## Containers over host binaries

This reverses the decision above. What forced it was not the application but the *edge*: one host should be able to serve several applications, and a Caddy that owns `:443` as a host process can only ever front one. Making the edge a shared container — reachable by every application on a dedicated network, publishing the only host port — is what makes a second application on the same machine possible at all.

Once the edge is a container, the rest follows. A containerized edge cannot dial another process's `127.0.0.1`, so the application has to be reachable on a network; once it is, the loopback-bind security argument (below, under Consequences) needs replacing rather than relaxing, and `DINCHY_TRUSTED_PROXIES` is that replacement. And a shared edge can read no application's files, so the compiled UI needs its own container rather than a directory.

**What the earlier reasoning got right, and what it cost.** The host-native design avoided granting anything: a process on the host simply *has* `/proc`, a PTY, and the container socket. The container has to be handed each one, and `deploy/quadlet/dinchy.container` is where those grants are visible and reviewable. The container socket is the significant one — it is root-equivalent on the host — and it is mounted for exactly the reason the original decision cited, that the management plane manages user containers. The trade is that the grant is now written down instead of implied.

**Consequence:** distribution is four small images rather than two binaries — the shared edge, the API, the compiled UI, and the datastores. A Go toolchain and network access are still needed to *build* them, and still not needed on the host that runs them. `xcaddy` remains available, but a plugin is better added as a blank import in `cmd/caddy/main.go`, where `go.mod` pins it and the image build picks it up with no second lockfile to keep honest.

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
3. **Plugin flexibility** — homelab users want Caddy plugins (`caddy-cloudflare` for DNS-01 challenges behind NAT, `caddy-security` for auth middleware, etc.). With embedded Caddy, a plugin would have to be linked into the management plane itself, and its dependency tree would ship with every Dinchy binary. With a sidecar, the operator builds their own with `xcaddy` and Dinchy never knows.

The sidecar is controlled entirely via Caddy's [JSON Admin API](https://caddyserver.com/docs/api) at `localhost:2019`. Dinchy is the single source of truth — routes are stored in PostgreSQL and pushed to Caddy on change. No Docker labels, no config file templating, no process reloads.

### One push at startup, and then hands off

Dinchy replaces Caddy's **entire** configuration exactly once, at startup, and never writes again. The only routes are the panel's and they are fixed for the life of the process, so nothing is lost by replacing everything.

There is deliberately no drift job. On a self-hosted box the operator owns the machine: a management plane that re-asserts its own view every minute silently discards whatever they changed, and it does so at a moment they are not watching. Setting the proxy up correctly is Dinchy's job; keeping it that way is theirs. Restoring a known-good configuration from the database is a thing to offer *on request*, later — not a background loop.

The cost is that a push failing at startup is not retried. Startup covers the race — Caddy's unit is ordered first and Dinchy waits a few seconds for its admin endpoint — but an outage past that is reported and left alone. Readiness probes Caddy live rather than remembering the last push, so the panel still tells an operator the proxy is down.

That stops being true as soon as routes come and go. A full configuration reload makes Caddy close active streaming connections ([caddy#6420](https://github.com/caddyserver/caddy/issues/6420), [#7222](https://github.com/caddyserver/caddy/issues/7222)). The web terminal and live log viewer are WebSockets behind Caddy, so reloading on every routing change would drop one operator's terminal because another added a domain — and in development it would kill Vite HMR on every save. Startup is still the exception: there are no such connections, and a full load is the only way to guarantee Caddy matches PostgreSQL after a restore or a restart.

The targeted path is therefore deferred rather than rejected. When it lands it needs three things, none of which exist yet: routes tagged with a stable `@id` so they are addressable at `/id/<id>` without knowing their index; the panel hostname reserved against other sources; and disjoint route match sets, so appending a route can never change which route wins a request — an exact host that an existing wildcard would also match must be rejected as a conflict. Wildcard hosts are refused outright until that model is in place.

One invariant holds either way:

- **The configuration always contains an `admin` block.** `POST /load` replaces everything, and Caddy tears down the old admin endpoint on every load, so a document omitting it would destroy the very endpoint Dinchy pushes through — unrecoverable without editing files by hand.

TLS is *not* a reload concern: Caddy keeps its certificate cache in a process-level global and reloads managed certificates from storage, so a reload with unchanged hostnames makes zero ACME requests. The limit that is actually reachable is failed authorizations per hour, which is why a domain whose DNS does not resolve is quarantined rather than retried indefinitely.

### Caddy is the validator, except for upstreams

Dinchy screens almost nothing before pushing. Caddy provisions the whole resulting configuration and rolls back to the previous one if anything fails, so a bad module identifier or a malformed structure comes back as a rejection with Caddy's own explanation — duplicating that in Go buys worse messages and a second thing to keep current across Caddy releases. That rollback is also what keeps one tenant's bad push from being every tenant's outage, and it is pinned by a contract test that asserts the other tenant is still serving afterwards.

Three gaps make Dinchy check anything at all:

- **Duplicate host + path.** Caddy accepts it, stops at the first terminal match, and leaves the losing route silently unreachable. Dinchy rejects the pair instead — but only within its own slice. A host claimed by *two different tenants* is detected by nobody, because each validates only what it owns; first-in-array wins and the loser is silently unreachable. Finding that needs a read of the whole running configuration, which no tenant does today.
- **Reverse-proxy dial addresses.** Caddy does *not* parse these at load time — `not a host:8080` loads with a 200 and every request through the route 502s (pinned by a contract test). Nothing composing a Route from user input can rely on Caddy to catch a typo here.

### Restart durability

The edge runs as `caddy run --resume --config /etc/caddy/base.json`. The API is the source of truth, so Caddy's autosaved configuration *is* the state to restore, and restarting the edge — after a plugin rebuild, or a crash — is safe with no management plane up. `--watch` is irrelevant for the same reason: there is no configuration file to watch.

Two details are worth pinning because both have surprised:

- **`--resume` overrides `--config`.** Whenever an autosave exists Caddy ignores the base file, warning that it did. So the base file seeds the *first* start and nothing after it, which means editing it does nothing to a running edge until the autosave is removed.
- **Every admin write autosaves the whole resulting document**, not just the part written. That is what makes the shared edge durable across a restart: one application's push preserves every other tenant's routes on disk, and the edge comes back serving all of them with nothing running to re-push.

The base file is a contract rather than a convenience: Caddy refuses to traverse into a path whose parents are absent, so the server and the policy array a tenant addresses have to exist before it can write into them. `deploy/caddy/README.md` enumerates the invariants.

### Caddy issues every certificate, development included

Dinchy loads no certificate from disk anywhere. In production Caddy obtains them over ACME; in development it signs them itself with its built-in local CA (`DINCHY_CADDY_TLS_ISSUER=internal`), per host, on demand. `caddy trust` installs that CA's root once, which is the same one-time cost `mkcert -install` used to carry.

That removes a surprising amount of machinery. Loading a certificate is not sufficient on its own: for an SNI Caddy is not managing, it needs a `tls_connection_policy` selecting that certificate by tag, or it aborts the handshake with a TLS internal error — so a file-cert design has to emit tagged certificates and matching policies. When every certificate is Caddy-managed, Caddy adds the policy itself and none of that exists. A new development hostname also needs no regeneration, where a wildcard mkcert certificate did.

mkcert survives for Mailpit alone, which serves STARTTLS from a cert file or not at all and cannot generate one. Development therefore has two local authorities; production has neither.

### Consequences

- Dinchy listens **plaintext** in every environment, development included. `DINCHY_TLS_*` no longer exists; there is exactly one TLS terminator and one place automatic HTTPS can live.
- Caddy owns HSTS, because the responses that most need it — the HTTP-to-HTTPS redirect and upstream-failure pages — are generated by Caddy and never reach Dinchy.
- **The app carries no transport check of its own.** It has no notion of whether a request was secure, and no endpoint rejects one for being plaintext. The edge is the only intended ingress and serves only HTTPS.

  This used to rest on a loopback bind, and the reasoning is worth keeping because it explains why the replacement looks the way it does. A per-request trusted-proxy check was removed once as unable to ever fire: with a loopback listener every peer that can connect *is* loopback, so it always answered "trusted", and it was blind to the threat it was written for, since a host-networked container reaching the host's `127.0.0.1` looks identical to Caddy.

  Containerizing the edge invalidated the premise, not the analysis. The listener now faces `caddy-edge`, where other containers live, so a trusted-proxy check *can* fire and is the only thing that distinguishes the edge from a co-tenant. It came back — but as a set of prefixes rather than a single "is it loopback" question, and gating exactly one thing (see below) rather than feeding forgeable in-app 403 guards.

- **The client IP comes from `X-Forwarded-For`, and `DINCHY_TRUSTED_PROXIES` is what makes it trustworthy.** The value is persisted to `audit_logs.ip_address` and `sessions.ip_address`, so reading the connection's own address instead would record the edge for every request — that half is a functional dependency. Honoring the header *only from a trusted peer* is the security half: anything else on the edge's network could otherwise choose what the audit log records about it. The edge replaces the header rather than appending, so there is no chain to walk and a trusted peer's value is the client address.

  The invariant is enforced once, at startup, and it is the pair rather than either half: `config.Load` **rejects a listener reachable beyond loopback when only loopback is trusted.** Such a deployment is not forgeable, but it would silently record the edge's own address for every request — wrong in a way no operator would ever see. The default trust set is loopback alone, which is exactly what the old loopback-only listener guaranteed, so a host-native deployment behaves as it always did.

- **The edge serves no files; Dinchy is API-only.** One hostname is split by path: `/api/*` reaches the Go process, everything else the compiled SPA — served by its own container in production and by the Vite dev server in development, and *proxied* either way. A shared edge can read no single application's files, so file serving left the routing model entirely: `caddy.Route` has no static-file mode any more, and the single-page fallback lives in the image that holds the files. Asset requests never enter a Go goroutine, and — more importantly — the browser stays same-origin, so the session and CSRF cookies remain `SameSite=Lax` and CORS is never in play. A cross-origin split would have required `SameSite=None`, a different CSRF strategy, and an explicit `connect-src`.

  The consequence for **Content-Security-Policy** is easy to get wrong: CSP is a document policy, delivered on the HTML response. Since Dinchy no longer serves that document, its policy governs JSON only and is correspondingly strict — `default-src 'none'` — because a JSON response has no legitimate reason to load a script or a frame. The document policy naming `script-src`, `style-src` and `connect-src` belongs to whoever serves the HTML: Caddy in production, Vite in development. That is also why Dinchy has no development CSP variant any more; the relaxations hot reloading needs are Vite's to declare.

  Path ordering is correctness, not tidiness: Caddy stops at the first terminal match, so the `/api` route must precede the catch-all. `orderRoutes` sorts longest-prefix-first for that reason.

- **The edge is the only place forwarded headers are set**, and that is one half of the invariant above. The generated configuration deletes `X-Real-IP`, `True-Client-IP` and `Forwarded`, which Caddy does **not** do by default — it passes them through untouched. Common Go middleware prefers those two over `X-Forwarded-For`, so the deletion is what stops a forged value from becoming the recorded client IP. It is covered by the contract test in `internal/platform/caddy` because nothing else would notice if it regressed.
- The proxied `Host` header is never rewritten: CORS origin checking and cookie scoping both depend on the original value.

---

## Caddy over Traefik/Nginx

**vs Traefik:** Traefik's Docker label discovery model makes Docker labels the source of truth for routing — competing with Dinchy's own state. Higher memory usage (50-200MB idle vs Caddy's ~30MB). Traefik has also benchmarked slower than Caddy in throughput tests.

**vs Nginx:** No API — routing changes require config file generation + `nginx -s reload`, which is a signal with no synchronous error channel, so a bad config surfaces in a log rather than to the caller that caused it. HTTPS requires certbot as a separate tool with its own cron job. Config syntax is a separate DSL to template correctly. Cannot be embedded in Go if needed later.

**Caddy wins** because: JSON Admin API allows atomic route changes from Go code with zero downtime; automatic HTTPS with no certbot; ~30MB idle memory; written in Go.

---

## systemd + OpenRC (not Docker for supervision)

> **Partly superseded.** systemd is still the supervisor; it now supervises containers through
> podman quadlets rather than host binaries. See "Containers over host binaries".

Using Docker to run Dinchy itself was considered and initially rejected because:
- Web terminal via PTY requires the process to be on the host, not inside a container
- Reading host system metrics (`/proc/meminfo`, disk usage) is simpler from the host
- Docker-in-Docker for managing user containers adds a layer of indirection

Each is a *grant* rather than a blocker, and containerizing the edge made paying them worthwhile — a host-process Caddy owning `:443` can front only one application. The grants are explicit in `deploy/quadlet/dinchy.container`, which is the improvement: what the management plane can reach is now reviewable rather than implied by being a host process.

**systemd remains the supervisor**, through quadlets: a `.container` file generates a `.service` unit, so ordering, restart policy and `OOMScoreAdjust` all still work, and the unit names still resolve — `caddy.container` generates `caddy.service`, which is what `dinchy.container` orders itself after. It covers 95%+ of homelab host OSes (Debian, Ubuntu, Fedora, Arch, Proxmox, RHEL). OpenRC coverage for Alpine and Gentoo is the cost of this change: quadlets are systemd-specific.

`OOMScoreAdjust=-1000` in the unit prevents the Linux OOM killer from ever targeting the management plane, even during a runaway build.

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
