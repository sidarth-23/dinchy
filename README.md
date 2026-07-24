# Dinchy

A lightweight, self-hosted deployment manager for homelabs and small teams. A better alternative to Dokploy and Coolify — polished UI, low memory footprint, first-class SSO, and a build system that doesn't crash your server.

## Why Dinchy?

| | Dokploy | Coolify | **Dinchy** |
|---|---|---|---|
| Idle memory | 350-600MB | 500-700MB | **< 200MB target** |
| API design | tRPC | REST | **REST** |
| SSO out of the box | No | Partial | **Yes** |
| Build safety | None | None | **Memory guardian** |
| Distribution | Docker | Docker | **Single binary** |
| Tech stack | Next.js + PostgreSQL + Redis | Laravel + PostgreSQL | **Go + PostgreSQL** |

## Architecture

```
                    User traffic (80/443)
                          ↕
┌──────────────────┐  localhost:2019  ┌────────────────────┐
│  Dinchy binary   │ ── Admin API ──→ │  Caddy (sidecar)   │
│                  │                  │  - Reverse proxy    │
│  - REST API      │                  │  - Auto HTTPS       │
│  - React UI      │                  └────────────────────┘
│  - WebSocket     │
│  - Terminal (PTY)│                  ┌────────────────────┐
│  - PostgreSQL    │ ── Docker/Podman ─→│  User containers   │
└──────────────────┘                  └────────────────────┘
        ↑
  systemd / OpenRC
```

Single-node, single binary. Caddy runs alongside as a sidecar and handles all routing and HTTPS.

## Tech Stack

- **Backend:** Go — single binary, no runtime dependencies
- **Database:** PostgreSQL — single supported datastore
- **Frontend:** React + Vite + Tanstack Router + Shadcn/ui — statically served from the binary via `embed.FS`
- **Reverse proxy:** Caddy — programmatically controlled via JSON Admin API, automatic HTTPS
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

This project uses [mise](https://mise.jdx.dev/) to manage all tooling. First-time setup:

```bash
mise install               # install pinned Go, Bun, Node, dlv, mkcert, and dev tools
cp .env.example .env       # your local config (gitignored) — edit as needed
mise run dev:certs         # mkcert: install a trusted local CA + issue app/infra certs (one-time; needs sudo)
mise run infra:up          # start Postgres + Redis + Mailpit (all over TLS)
mise run db:migrate        # apply database migrations
mise run dev               # run the backend — colored, human-readable logs
```

Dinchy serves HTTPS directly, exactly like production: the Go server terminates TLS on
**https://localhost:8443** using a locally-trusted mkcert certificate (no reverse proxy).
Auth endpoints reject plaintext, so open **https://localhost:8443**. Mailpit enforces
STARTTLS + auth like production (the app connects with mandatory STARTTLS), while Postgres
and Redis run plaintext on loopback in dev — TLS on loopback datastores isn't worth the
rootless-container friction locally, and production configures it separately.
`mise run dev:certs` runs `mkcert -install` once, adding the local CA to your system and
browser trust stores, so the app's HTTPS and Mailpit's cert are trusted with no warning.

`.env.example` is the only committed template. Copy it to `.env` (gitignored) for your
working config — including `DINCHY_LOG_FORMAT=text`, which produces colored, human-readable
logs in a terminal, and the Postgres DSN. An optional `.env.local` (also gitignored) can
hold personal overrides; mise loads both, with `.env.local` winning over `.env`.

Local Postgres + Redis run from `compose.yaml` via **podman-compose** (a mise-managed tool
that drives podman directly — no Docker daemon or socket). The host ports are **configurable
via env**: `compose.yaml` interpolates `DINCHY_POSTGRES_PORT` / `DINCHY_REDIS_PORT` from
`.env` (default `5433` / `6380`, to avoid clashing with anything already on `5432` / `6379`).
Keep the DSN / `DINCHY_REDIS_ADDR` ports in sync with those keys.

> **WSL2 note:** if `mise run infra:up` fails with `netavark: nftables error`, uncomment
> `NETAVARK_FW=none` in your `.env` (nftables is unusable on some WSL2 kernels; rootless
> port forwarding still works without it). On native Linux, leave it unset so podman uses
> its default nftables firewall.

Everyday tasks:

```bash
mise run dev          # run the Go backend in development mode (HTTPS on :8443)
mise run dev:certs    # (re)issue local TLS certs for the app + infra via mkcert
mise run test         # run the test suite
mise run lint         # run the linter
mise run build        # build the production binary
mise run generate     # regenerate generated Go code and sqlc queries
mise run db:migrate   # run database migrations against DINCHY_POSTGRES_DSN
mise run infra:up     # start local Postgres + Redis + Mailpit (podman-compose)
mise run infra:down   # stop local infra
mise run infra:logs   # follow infra logs
```

`mise run infra:up` also starts **Mailpit**, a local mail catcher, so the email flows
(invitations, password reset) work without a real mail server. Mailpit is configured like a
production relay — **STARTTLS is mandatory and SMTP auth is required** — so the app connects
over TLS (verifying Mailpit's mkcert cert) with the `DINCHY_SMTP_USERNAME` /
`DINCHY_SMTP_PASSWORD` from `.env.example`, which must match `MP_SMTP_AUTH` in `compose.yaml`.
Keep the `DINCHY_SMTP_*` / `DINCHY_PUBLIC_BASE_URL` block from `.env.example` in your `.env`;
outbound mail is caught by Mailpit rather than sent for real, and you can read it in the web
mailbox at **http://localhost:8025**. Without those vars SMTP stays disabled and the
invite/reset endpoints report email as not configured.

> Optional one-time podman hardening: `sudo loginctl enable-linger $USER` gives your user a
> persistent systemd session so podman uses the systemd cgroup manager and the rootless
> `cgroupfs`-fallback warnings disappear. Not required — the infra works without it.

### Debugging in Zed

`.zed/debug.json` defines a **dinchy (dev)** configuration that launches `cmd/dinchy` under
Delve (`dlv`, installed by `mise install`). Set a breakpoint and start that configuration;
its environment is loaded entirely from `.env` + `.env.local` via `envFile`, so nothing
sensitive is committed and edits to those files take effect on the next launch.
`.zed/tasks.json` also exposes the mise tasks (dev, infra up/down, migrate, test, …) in
Zed's task runner (`task: spawn`).

> **Open this repo as the Zed project root**, not a parent folder. The debug config and
> tasks resolve paths via `$ZED_WORKTREE_ROOT`, which Zed sets to the folder you opened. If
> you open an ancestor (e.g. a multi-project workspace), `$ZED_WORKTREE_ROOT` points there
> and the debugger fails with `package cmd/dinchy is not in std`. Open the directory that
> contains this `README.md`.

When the frontend exists:

```bash
mise run web:install  # install frontend dependencies (via Bun)
mise run web:dev      # run the frontend dev server (Bun runtime)
mise run web:build    # build production frontend assets
```

Bun is the primary JS runtime for this project. Node is available via mise for compatibility, but all frontend tasks run through Bun.

The database is PostgreSQL only. `DINCHY_POSTGRES_DSN` is supplied by `.env.local` for local dev; set it explicitly in other environments.

## License

MIT
