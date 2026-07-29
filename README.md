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

This project uses [mise](https://mise.jdx.dev/) to manage all tooling. First-time setup:

```bash
mise install               # install pinned Go, Bun, Node, dlv, mkcert, xcaddy
cp .env.example .env       # your local config (gitignored) — edit as needed
mise run caddy:trust       # trust Caddy's local CA, so dev HTTPS has no warning (one-time; sudo)
mise run dev:certs         # mkcert: Mailpit's STARTTLS cert (one-time; needs sudo)
mise run infra:up          # start Postgres + Redis + Mailpit
mise run db:migrate        # apply database migrations
mise run caddy:dev         # terminal 1: Caddy, which terminates TLS and routes
mise run dev               # terminal 2: the API — colored, human-readable logs
mise run web:dev           # terminal 3: Vite, once web/ exists
```

> **Upgrading an existing checkout?** Merge the `--- Server ---` and `--- Caddy ---` blocks
> from `.env.example` into your `.env`. `DINCHY_ADDR` moved from `:8443` to `:8080`, the
> `DINCHY_TLS_*` variables are gone, and `DINCHY_CADDY_AUTOMATIC_HTTPS` / `_CERT_FILE` /
> `_KEY_FILE` / `_RECONCILE_INTERVAL` were replaced by `DINCHY_CADDY_TLS_ISSUER=internal`.
> Run `mise run caddy:trust` once; Caddy now issues its own dev certificates.

**Caddy terminates all TLS, exactly like production.** Open **https://localhost:8443** —
Caddy serves it with a certificate it signed itself, per host, using its built-in local CA
(`mise run caddy:trust` puts that CA's root in your trust store, so there is no warning). One hostname, split by path:
`/api/*` goes to the Go process on `127.0.0.1:8080`, and everything else goes to the
frontend — the Vite dev server in development, `web/dist` read straight off disk in
production. The Go process never serves the UI, so requesting `/` from it returns 404 by
design.

Because both halves share a hostname the browser stays same-origin, which is what keeps the
session and CSRF cookies working on `SameSite=Lax` without CORS being involved.

Hitting `:8080/api/...` directly works and is useful for debugging; the app has no
transport check of its own, and `DINCHY_ADDR` is required to be a loopback address so the
network cannot reach it. Dinchy refuses to start if it isn't.

**Start Caddy before Dinchy.** Caddy comes up with only its admin endpoint, and Dinchy
pushes the routes to it once, at startup. It waits a few seconds for Caddy if it has to,
then gives up and logs the failure — there is no background job that converges later, on
purpose: once the proxy is configured, changes you make to it are yours to keep. If you do
start them the wrong way round, restart Dinchy.

Deployments will be reachable at **`https://<slug>.dinchy.localhost:8443`**, each with its
own certificate issued on demand — nothing to regenerate when a name is added. Those names
resolve to loopback with no `/etc/hosts` entry through `nss-myhostname` and the browsers'
built-in `.localhost` handling. Two caveats: Safari does not implement it, and Go's own
resolver bypasses NSS entirely — a Go client needs an explicit `/etc/hosts` entry even
though curl and browsers do not.

Mailpit enforces STARTTLS + auth like production (the app connects with mandatory
STARTTLS), while Postgres and Redis run plaintext on loopback in dev — TLS on loopback
datastores isn't worth the rootless-container friction locally, and production configures
it separately. `mise run dev:certs` exists for Mailpit alone: it serves STARTTLS from a
cert file or not at all, and cannot generate one, so `mkcert -install` puts a root in your
system store that the Go SMTP client can verify against. Caddy needs nothing from it —
`mise run caddy:trust` installs its own CA root instead. Development therefore has two
local authorities; production has neither, because it uses ACME with a real domain.

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
mise run caddy:dev    # run Caddy FIRST: terminates TLS on :8443 and routes to the backend
mise run dev          # run the Go backend (plaintext on 127.0.0.1:8080, fronted by Caddy)
mise run caddy:build  # build Caddy from cmd/caddy into tmp/caddy
mise run caddy:version # the pinned Caddy version, for building a plugin with xcaddy
mise run caddy:trust  # install Caddy's local CA root into the system trust store
mise run caddy:modules # list the Caddy modules compiled into the binary
mise run dev:certs    # (re)issue Mailpit's STARTTLS cert via mkcert
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

### Extending Caddy with plugins

Caddy is a sidecar rather than a library precisely so you can swap in your own build.

The baseline build is [`cmd/caddy/main.go`](cmd/caddy/main.go): upstream Caddy, no plugins.
It exists to **pin the version** — `go.mod` and `go.sum` lock it, so `mise run caddy:build`
produces the same binary on every machine with no manifest of our own to maintain.

Plugins are yours, and `xcaddy` compiles them. Point it at the same pinned version so your
build and the baseline are the same Caddy:

```bash
xcaddy build "$(mise run caddy:version)" \
  --with github.com/caddy-dns/cloudflare \
  --output tmp/caddy
mise run caddy:modules    # confirm what actually got compiled in
```

That last step is not optional bookkeeping: `caddy list-modules` is the only reliable
answer to "is the plugin loaded", because a Go module can resolve and build while
registering no Caddy module at all.

The most common addition is a DNS provider. It lets Caddy answer ACME DNS-01 challenges and
so issue certificates for a homelab behind NAT, with no inbound port 80.

Note that an xcaddy build is pinned only by the version you pass it — the plugin's own
version is resolved at build time and recorded nowhere in this repo. If you want a plugin
pinned and reviewable, add a blank import to `cmd/caddy/main.go` and `go get` it instead;
`go.mod` then covers it too.

This is the one place Dinchy trades away a single-artifact install — building Caddy needs a
Go toolchain and network access.

In production, install the built binary at the path
[`deploy/systemd/caddy.service`](deploy/systemd/caddy.service) runs, and restart Caddy. The
restart is safe: it runs with `--resume` and restores the routes Dinchy pushed, so no
configuration is lost and Dinchy does not need to be running.

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
