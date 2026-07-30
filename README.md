# Dinchy

A lightweight, self-hosted deployment manager for homelabs and small teams. A better alternative to Dokploy and Coolify — polished UI, low memory footprint, first-class SSO, and a build system that doesn't crash your server.

## Why Dinchy?

| | Dokploy | Coolify | **Dinchy** |
|---|---|---|---|
| Idle memory | 350-600MB | 500-700MB | **< 200MB target** |
| API design | tRPC | REST | **REST** |
| SSO out of the box | No | Partial | **Yes** |
| Build safety | None | None | **Memory guardian** |
| Distribution | Docker | Docker | **Host binaries, no container runtime** |
| Extend the proxy | No | No | **Yes — recompile Caddy with any plugin** |
| Tech stack | Next.js + PostgreSQL + Redis | Laravel + PostgreSQL | **Go + PostgreSQL** |

## Architecture

```
                       User traffic (80/443)
                                ↕
                     ┌─────────────────────┐
                     │  Caddy (sidecar)    │  terminates ALL TLS
                     │  - Reverse proxy    │  - auto HTTPS (ACME)
                     │  - Static files     │  - HSTS
                     └─────────────────────┘
              /api/*  │        │  /*        │  container ports
                      ▼        ▼            ▼
┌──────────────────┐  ┌──────────────┐  ┌────────────────────┐
│  Dinchy binary   │  │  web/dist    │  │  User containers   │
│  (plaintext,     │  │  (React SPA) │  └────────────────────┘
│   loopback only) │  └──────────────┘            ▲
│  - REST API      │ ── Docker/Podman ────────────┘
│  - WebSocket     │
│  - Terminal (PTY)│          ┌────────────────────┐
│  - PostgreSQL    │ ── JSON →│ Caddy Admin API    │
└──────────────────┘  routes  │ 127.0.0.1:2019     │
        ↑                     └────────────────────┘
  systemd / OpenRC
```

Single-node. Caddy handles all routing and HTTPS; Dinchy never terminates TLS and listens
on loopback only. One hostname is split by path — `/api/*` reaches the Go process and
everything else is the compiled UI, which Caddy reads straight from disk. That keeps the
browser same-origin, so session and CSRF cookies stay `SameSite=Lax` and CORS never comes
into it, while asset requests never occupy a Go goroutine.

Routes live in PostgreSQL and are pushed to Caddy's JSON admin API once, at startup. Dinchy
does not re-assert afterwards: an operator who adjusts the running proxy keeps that change.
Addressing one route at a time — so adding a domain does not close every WebSocket,
including other users' terminals — is what the admin API is for, and lands with the
deployment routing that needs it.

## Tech Stack

- **Backend:** Go — one static binary, no runtime dependencies
- **Database:** PostgreSQL — single supported datastore
- **Frontend:** React + Vite + Tanstack Router + Shadcn/ui — served directly by Caddy from `web/dist`, never through the Go process
- **Reverse proxy:** Caddy — programmatically controlled via its JSON Admin API, automatic HTTPS, and recompilable with any Caddy plugin
- **Process supervision:** systemd (Debian, Ubuntu, Fedora, Arch, Proxmox) + OpenRC (Alpine, Gentoo)

## Features (v1)

- **Three deployment modes:**
  - Docker Compose — paste a compose file, Dinchy manages the lifecycle
  - Container image — specify an image + ports + env vars, done
  - Git + Dockerfile — point at a repo, Dinchy builds and deploys
- **Private registry support** — configure credentials for Docker Hub, ghcr.io, self-hosted registries
- **Build Resource Guardian** — pre-flight memory checks, build queue, memory watchdog, graceful abort instead of OOM crashes
- **Authentication:** email/password, OAuth/OIDC (Google, GitHub, Authentik, Authelia, Keycloak), 2FA (TOTP), forgot password, first-user setup
- **Authorization:** Casbin RBAC — Admin / Deployer / Viewer roles
- **Automatic HTTPS** — via Caddy + Let's Encrypt, zero config
- **Web terminal** — browser-based shell access without SSH keys
- **Log viewer** — real-time streaming, historical search (SQLite FTS5), syntax-highlighted by log level
- **Secrets management** — AES-256-GCM encrypted env vars, masked in UI
- **Monitoring:** container CPU/memory sparklines, HTTP health checks, uptime graphs
- **Alerts:** webhook-based (works with ntfy, Slack, Discord, email, anything)
- **Deploy history + rollback** — one-click rollback to any previous state
- **Auto-updates** — updater binary with checksum verification and auto-rollback on failure

## Roadmap

See [docs/plan.md](docs/plan.md) for the full phased implementation plan.

Detailed Phase 1 implementation docs live in [docs/implementation/phase-1/](docs/implementation/phase-1/README.md).

**v2+:**
- Multi-node support (gRPC master/agent architecture)
- Plugin ecosystem (container-based plugins)
- App template store
- Podman support
- External secret managers (Vault, SOPS)
- Scheduled jobs

## Installation

> Coming in Phase 8.

```bash
curl -fsSL https://dinchy.com/install.sh | bash
```

## Development

Two tools divide the work. [mise](https://mise.jdx.dev/) pins the toolchain and installs
nothing else; [Task](https://taskfile.dev/) runs every command. `task --list` is the index of
what you can run, and `task --summary <task>` explains any one of them in depth — this README
covers how the pieces fit together, not what each command does.

### Prerequisites

- **mise**, which brings the rest of the toolchain with it.
- **podman**, for the local datastores. Rootless is fine; there is no Docker daemon anywhere.

### Bootstrap

```bash
mise trust && mise install   # the pinned toolchain, and nothing else
task setup                   # this machine: .env, git hooks, frontend deps, TLS trust
```

Run `task setup` from a terminal: installing a CA root needs your password, so it asks for one.
Nothing is hung off `mise install` — installing tools is deliberately free of side effects,
which is what lets setup be interactive rather than defensively silent. Every step tests for
its own result first, so running setup again reports what is already true, asks for nothing,
and is the way to repair a half-finished machine.

Then edit `.env` if you want anything other than the defaults.

### The session loop

Four independent processes, in this order. They are separate on purpose: only the backend
restarts in a normal edit-debug cycle, so the edge, the datastores and the frontend keep their
state.

| # | Process | Start it with | Zed task | Listens on |
|---|---------|---------------|----------|------------|
| 1 | Postgres + Redis + Mailpit | `task dev:up` | **Dev: start infra** | 5432 / 6380 / 1025 + 8025 |
| 2 | Caddy — terminates TLS | `task caddy:dev` | **Dev: run Caddy edge** | `:8443` HTTPS, `127.0.0.1:2019` admin |
| 3 | Vite dev server | `task web:dev` | **Dev: run web UI** | `127.0.0.1:3000` |
| 4 | Go backend | `task dev` | **Backend: API (Delve)** — debugger | `127.0.0.1:8080` plaintext |

Step 1 brings the containers up, waits until Postgres actually accepts connections, and
migrates, so nothing downstream races a cold container. Steps 2, 3 and 4 each want their own
terminal. Then open **https://localhost:8443**.

**The order matters.** Dinchy pushes its routes to Caddy once, at startup, waits a few seconds
if it has to, and then gives up — there is deliberately no job that converges later, so
changes you make to the running proxy stay yours (see
[docs/architecture.md](docs/architecture.md#one-push-at-startup-and-then-hands-off)). If you
start them the wrong way round, restart Dinchy. Caddy tolerates the backend coming and going:
each launch re-pushes the routes, and Caddy keeps serving in between.

Caddy terminates all TLS here exactly as it does in production, and splits one hostname by
path: `/api/*` reaches the Go process, everything else reaches Vite. Requesting `/` from the
Go process returns 404 by design. Hitting `:8080/api/...` directly is fine for debugging —
`DINCHY_ADDR` is required to be a loopback address, and Dinchy refuses to start otherwise,
which is what keeps the network off the plaintext listener.

Deployments will be reachable at **`https://<slug>.dinchy.localhost:8443`**, each with a
certificate Caddy issues on demand — nothing to regenerate when a name is added. Those names
resolve to loopback with no `/etc/hosts` entry, through `nss-myhostname` and the browsers'
built-in `.localhost` handling. Two caveats: Safari does not implement it, and Go's own
resolver bypasses NSS entirely, so a Go client needs an explicit `/etc/hosts` entry even
though curl and browsers do not.

### Local configuration

**`.env` is the single source of truth**, and `.env.example` is the only committed template.
`task setup` copies one to the other; both Task and the Zed debugger (`envFile` in
`.zed/debug.json`) load exactly that file, so there is one place to look when a value looks
wrong. `DINCHY_LOG_FORMAT=text` is worth keeping — it produces colored, human-readable logs.

The binary itself never reads `./.env`. `config.Load` resolves `DINCHY_ENV_FILE`, then
`~/.config/dinchy/dinchy.env`, then `/etc/dinchy/dinchy.env` (see
[`internal/config/env.go`](internal/config/env.go)), which is why `./dinchy` run by hand
outside Task gets none of your local configuration. Use `task dev` or the debugger.

`DINCHY_EXPOSE_INTERNAL_ERRORS=true` adds the internal code, the cause chain (including SQL
errors) and the error metadata to every API response — the fastest way to see why a request
failed without stepping through it. It leaks internal detail by design, so it belongs in a
local `.env` only.

### Local infrastructure

Postgres, Redis and Mailpit run from `compose.yaml` under podman-compose, which drives podman
directly — no daemon, no socket. Host ports come from `.env`: `DINCHY_POSTGRES_PORT` and
`DINCHY_REDIS_PORT`, defaulting to `5432` and `6380`. Redis is off its usual port because a
system Redis commonly holds it; move Postgres the same way if you already run one, keeping the
DSN and `DINCHY_REDIS_ADDR` in step.

**Mailpit** catches the invitation and password-reset mail so those flows work without a real
mail server, and it is configured like a production relay — STARTTLS mandatory, SMTP auth
required. The `DINCHY_SMTP_USERNAME` / `DINCHY_SMTP_PASSWORD` in your `.env` must match
`MP_SMTP_AUTH` in `compose.yaml`, or SMTP stays disabled and the invite and reset endpoints
report email as not configured. Read the caught mail at **http://localhost:8025**.

A major Postgres version bump leaves behind a data directory the new version refuses to start;
`task infra:reset` is the way out, and it will ask before deleting the volumes.

> **WSL2:** if starting infra fails with `netavark: nftables error`, uncomment `NETAVARK_FW=none`
> in your `.env` — nftables is unusable on some WSL2 kernels, and rootless port forwarding still
> works without it. On native Linux, leave it unset.

> Optional podman hardening: `sudo loginctl enable-linger $USER` gives your user a persistent
> systemd session, so podman uses the systemd cgroup manager and the rootless `cgroupfs`
> warnings disappear. Not required.

### Local TLS and trust

Development has two local certificate authorities because they solve different problems, and
production has neither — it uses ACME with a real domain. Caddy signs the app's certificates
with its own internal CA; mkcert issues Mailpit's, which Mailpit cannot generate for itself and
which the Go SMTP client verifies against the system trust store. The reasoning is in
[docs/architecture.md](docs/architecture.md#caddy-issues-every-certificate-development-included).

`task setup` installs both roots. Each goes into two independent stores: the system store,
which needs your password, and a per-user NSS database that Chrome and Firefox read *instead*
of the system one. A machine with only the system half looks correctly set up right up until
the browser rejects the page, so setup installs `certutil` and writes both. If certutil cannot
be installed on your distribution, `task tls:system` sets up the system half alone and you
accept a certificate warning in the browser.

Certificates and keys live in `$XDG_STATE_HOME/dinchy/certs` (usually
`~/.local/state/dinchy/certs`) — machine state, outside the repository, so a private key is
never in a working tree. `scripts/dev-tls.sh` holds the per-platform parts; `task tls` and its
subtasks own the ordering and the "already done" checks.

### Extending Caddy with plugins

Caddy is a sidecar rather than a library precisely so you can swap in your own build; the
reasoning is in [docs/architecture.md](docs/architecture.md#caddy-as-sidecar-not-embedded).
The baseline is [`cmd/caddy/main.go`](cmd/caddy/main.go) — upstream Caddy, no plugins, existing
only to **pin the version** in `go.mod` so `task caddy:build` is reproducible.

Plugins are yours, and `xcaddy` compiles them. Point it at the same pinned version:

```bash
xcaddy build "$(task caddy:version)" \
  --with github.com/caddy-dns/cloudflare \
  --output tmp/caddy
task caddy:modules    # confirm what actually got compiled in
```

That last step is not bookkeeping: `caddy list-modules` is the only reliable answer to "is the
plugin loaded", because a Go module can resolve and build while registering no Caddy module at
all. The most common addition is a DNS provider, which lets Caddy answer ACME DNS-01
challenges and so issue certificates for a homelab behind NAT with no inbound port 80.

An xcaddy build is pinned only by the version you pass it — the plugin's own version is
resolved at build time and recorded nowhere. For a plugin that is pinned and reviewable, add a
blank import to `cmd/caddy/main.go` and `go get` it instead, so `go.mod` covers it too. This is
the one place Dinchy trades away a single-artifact install: building Caddy needs a Go toolchain
and network access.

### Debugging in Zed

[`.zed/debug.json`](.zed/debug.json) defines three configurations, named after what they debug:

| Configuration | Debugs | Needs running first |
|---------------|--------|---------------------|
| **Backend: API (Delve)** | `cmd/dinchy` under `dlv` | infra (1), Caddy (2) |
| **Backend: tests in current package (Delve)** | the tests beside the file you have open | nothing |
| **Frontend: web UI (Chrome)** | `.tsx` sources in a real browser | infra (1), Caddy (2), Vite (3) |

Substitute **Backend: API (Delve)** for step 4 of the session loop: it is the only process you
restart while working. Its environment comes entirely from `.env` via `envFile`, so edits take
effect on the next launch. The test configuration deliberately does *not* load `.env` — the
tests build their own environment with `t.Setenv`, and a stray `DINCHY_*` value would mask a bug
rather than reveal one.

The Chrome configuration opens `https://localhost:8443` — the edge, not Vite directly, because
only through Caddy is the UI same-origin with `/api`. `webRoot` points at `web/`, which is how
Vite's source maps resolve back to files on disk, so a breakpoint set in a `.tsx` file binds to
the running app. Chrome launches with a throwaway profile, so no state carries between sessions.

[`.zed/tasks.json`](.zed/tasks.json) holds only the long-lived session processes and `task
check`. Everything else is `task --list` in a terminal, which cannot fall out of step with
`Taskfile.yml` the way a copy in the palette would.

> **Open this repo as the Zed project root**, not a parent folder. The debug configuration and
> the tasks resolve paths via `$ZED_WORKTREE_ROOT`, which Zed sets to the folder you opened. If
> you open an ancestor — a multi-project workspace, say — the debugger fails with `package
> cmd/dinchy is not in std`.

## License

MIT
