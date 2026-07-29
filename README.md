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

This project uses [mise](https://mise.jdx.dev/) to manage all tooling. One command sets up a
machine:

```bash
mise install    # tools, git hooks, .env, and local TLS trust — prompts for sudo once
```

Beyond installing the pinned toolchain, its postinstall hook runs `mise run dev:setup`, which
seeds `.env` from `.env.example` if you have none and puts both development CA roots into
every trust store on the machine — including the NSS database Chrome and Firefox read, which
is a separate store the system one does not cover (see
[`deploy/certs/setup-dev-tls.sh`](deploy/certs/setup-dev-tls.sh)). Every step tests for its own
result first, so later `mise install` runs are silent and ask for nothing. It declines to
prompt at all when it cannot — in CI, or a non-interactive shell — and tells you to re-run
`mise run dev:setup` from a terminal instead of failing the install.

Then edit `.env` if you want anything other than the defaults.

Then, every session, four independent processes. They are separate on purpose: only the
backend restarts in a normal edit-debug cycle, so the edge, the datastores and the frontend
keep their state.

| # | Process | Start it with | Zed task | Listens on |
|---|---------|---------------|----------|------------|
| 1 | Postgres + Redis + Mailpit | `mise run dev:up` | **Dev: start infra** | 5432 / 6380 / 1025 + 8025 |
| 2 | Caddy — terminates TLS | `mise run caddy:dev` | **Dev: run Caddy edge** | `:8443` HTTPS, `127.0.0.1:2019` admin |
| 3 | Vite dev server | `mise run web:dev` | **Dev: run web UI** | `127.0.0.1:3000` |
| 4 | Go backend | `mise run dev` | **Backend: API (Delve)** — debugger | `127.0.0.1:8080` plaintext |

`mise run dev:up` brings the containers up, waits until Postgres actually accepts
connections, and applies migrations — so nothing downstream races a cold container. Steps 2,
3 and 4 each want their own terminal, **in that order** (see "Start Caddy before Dinchy"
below). Then open **https://localhost:8443**.

> **Upgrading an existing checkout?** Re-copy `.env.example` over your `.env` rather than
> patching it: `DINCHY_ADDR` moved from `:8443` to `:8080`, the `DINCHY_TLS_*` variables are
> gone, `DINCHY_CADDY_AUTOMATIC_HTTPS` / `_CERT_FILE` / `_KEY_FILE` / `_RECONCILE_INTERVAL`
> were replaced by `DINCHY_CADDY_TLS_ISSUER=internal`, and the whole `--- Caddy ---` block is
> new. A variable the app no longer reads is ignored in silence, so a partially-merged `.env`
> fails as a production-shaped default (ACME on `:443`) rather than as an error. Then run
> `mise run dev:setup` once; Caddy now issues its own dev certificates. Local Postgres also
> moved to 18, which cannot start a data directory written by an older major version — run
> `podman volume rm dinchy_dinchy-pg` before `mise run dev:up` to reinitialize it.

**Caddy terminates all TLS, exactly like production.** Open **https://localhost:8443** —
Caddy serves it with a certificate it signed itself, per host, using its built-in local CA
(`mise install` puts that CA's root in your trust stores, so there is no warning). One hostname, split by path:
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
it separately. mkcert exists for Mailpit alone: it serves STARTTLS from a cert file or not at
all, and cannot generate one, so `mkcert -install` puts a root in your system store that the Go
SMTP client can verify against. Caddy needs nothing from it — it signs with its own internal CA
instead. Development therefore has two local authorities, both installed by `mise install`;
production has neither, because it uses ACME with a real domain.

**`.env` is the single source of truth for local configuration**, and `.env.example` is the
only committed template. Copy it to `.env` (gitignored) for your working config — including
`DINCHY_LOG_FORMAT=text`, which produces colored, human-readable logs in a terminal, and the
Postgres DSN. Both mise (`[env] _.file` in `mise.toml`) and the Zed debugger (`envFile` in
`.zed/debug.json`) load exactly that one file, so there is one place to look when a value
looks wrong.

The binary itself never reads `./.env`: `config.Load` resolves `DINCHY_ENV_FILE`, then
`~/.config/dinchy/dinchy.env`, then `/etc/dinchy/dinchy.env` (see
[`internal/config/env.go`](internal/config/env.go)). That is why `./dinchy` run by hand,
outside mise, gets none of your local config — use `mise run dev` or the debugger.

Local Postgres + Redis run from `compose.yaml` via **podman-compose** (a mise-managed tool
that drives podman directly — no Docker daemon or socket). The host ports are **configurable
via env**: `compose.yaml` interpolates `DINCHY_POSTGRES_PORT` / `DINCHY_REDIS_PORT` from
`.env`, defaulting to `5432` / `6380` — Redis is off its usual port because a system Redis
commonly holds it. Move Postgres the same way if you already run one locally, and keep the
DSN / `DINCHY_REDIS_ADDR` ports in sync with those keys.

> **WSL2 note:** if `mise run infra:up` fails with `netavark: nftables error`, uncomment
> `NETAVARK_FW=none` in your `.env` (nftables is unusable on some WSL2 kernels; rootless
> port forwarding still works without it). On native Linux, leave it unset so podman uses
> its default nftables firewall.

Everyday tasks:

```bash
mise run dev:up       # infra up, wait for Postgres, migrate — run this first
mise run caddy:dev    # then Caddy: terminates TLS on :8443 and routes to the backend
mise run web:dev      # then Vite on 127.0.0.1:3000, reached through Caddy
mise run dev          # then the Go backend (plaintext on 127.0.0.1:8080, fronted by Caddy)
mise run dev:setup    # one-time machine setup: .env + both CA roots (idempotent)
mise run dev:tls      # just the TLS half of dev:setup
mise run dev:env      # just the .env half of dev:setup
mise run caddy:build  # build Caddy from cmd/caddy into tmp/caddy
mise run caddy:version # the pinned Caddy version, for building a plugin with xcaddy
mise run caddy:trust  # raw `caddy trust`; needs Caddy already running — prefer dev:tls
mise run caddy:modules # list the Caddy modules compiled into the binary
mise run dev:certs    # reissue Mailpit's STARTTLS cert via mkcert (delete the pair first)
mise run test         # run the test suite
mise run lint         # run the linter
mise run build        # build the production binary
mise run generate     # regenerate generated Go code and sqlc queries
mise run db:migrate   # run database migrations against DINCHY_POSTGRES_DSN
mise run infra:up     # start local Postgres + Redis + Mailpit (podman-compose)
mise run infra:wait   # block until Postgres accepts connections on DINCHY_POSTGRES_DSN
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

[`.zed/debug.json`](.zed/debug.json) defines three configurations, named after what they
debug:

| Configuration | Debugs | Needs running first |
|---------------|--------|---------------------|
| **Backend: API (Delve)** | `cmd/dinchy` under `dlv` | infra (1), Caddy (2) |
| **Backend: tests in current package (Delve)** | the tests beside the file you have open | nothing |
| **Frontend: web UI (Chrome)** | `.tsx` sources in a real browser | infra (1), Caddy (2), Vite (3) |

The session loop, using the task runner (`task: spawn`) for the long-lived processes and the
debugger only for what you actually step through:

1. **Dev: start infra** — once per session
2. **Dev: run Caddy edge** — leave it running
3. **Dev: run web UI** — leave it running
4. **Backend: API (Delve)** — the only thing you restart while working

Caddy tolerates the backend coming and going: each launch re-pushes the routes, and Caddy keeps
serving between launches. The reverse is not true, which is why it starts first.

**Backend.** Its environment comes entirely from `.env` via `envFile`, so nothing sensitive is
committed and edits take effect on the next launch. The test configuration deliberately does
*not* load `.env` — the tests build their own environment with `t.Setenv`, and a stray
`DINCHY_*` value would mask a bug rather than reveal one. `program` is `$ZED_DIRNAME`, so it
debugs whichever package the open file belongs to.

**Frontend.** The Chrome configuration opens `https://localhost:8443` — the edge, not Vite
directly, because only through Caddy is the UI same-origin with `/api`. `webRoot` points at
`web/`, which is how the `/src/...` paths in Vite's source maps resolve back to files on disk;
breakpoints set in a `.tsx` file in Zed bind to the running app. Chrome launches with a
throwaway profile, so there is no cached state between sessions.

> Browser trust is its own store: Chrome and Firefox read certificates from an NSS database
> rather than the system one. `mise install` (or `mise run dev:setup`) installs `certutil` and
> puts both development roots there, which is what keeps this from opening on a certificate
> interstitial. If you skipped that step, the page will not load under the debugger.

Debugging the API directly is also fine — `curl 127.0.0.1:8080/api/...` skips Caddy entirely.
What you lose is the browser's same-origin view of the app, so anything cookie-dependent should
go through `https://localhost:8443`.

> `DINCHY_EXPOSE_INTERNAL_ERRORS=true` in `.env` adds the internal code, the cause chain
> (including SQL errors) and the error metadata to every API response — the fastest way to
> see why a request failed without stepping through it. It leaks internal detail by design,
> so it belongs in local `.env` only.

> **Open this repo as the Zed project root**, not a parent folder. The debug config and
> tasks resolve paths via `$ZED_WORKTREE_ROOT`, which Zed sets to the folder you opened. If
> you open an ancestor (e.g. a multi-project workspace), `$ZED_WORKTREE_ROOT` points there
> and the debugger fails with `package cmd/dinchy is not in std`. Open the directory that
> contains this `README.md`.

### Frontend

```bash
mise run web:install  # install frontend dependencies (via Bun)
mise run web:dev      # run the Vite dev server on 127.0.0.1:3000
mise run web:build    # build production frontend assets into web/dist
```

Bun is the primary JS runtime for this project. Node is available via mise for compatibility, but all frontend tasks run through Bun.

Vite's port is pinned in [`web/vite.config.ts`](web/vite.config.ts) with `strictPort`, not on
the command line: Caddy proxies to a fixed upstream (`DINCHY_DEV_PROXY_URL`), so silently
falling back to the next free port would show up as a blank page behind the proxy rather than
an error at startup. The same config points HMR's WebSocket at Caddy on `:8443`, because the
browser reaches Vite through the edge and never dials `:3000` itself.

The database is PostgreSQL only, and `DINCHY_POSTGRES_DSN` is the single knob — there are no
separate host/port/user variables. Local dev reads it from `.env`; every other environment
sets it explicitly.

## License

MIT
