# Dinchy

A lightweight, self-hosted deployment manager for homelabs and small teams. A better alternative to Dokploy and Coolify — polished UI, low memory footprint, first-class SSO, and a build system that doesn't crash your server.

## Why Dinchy?

| | Dokploy | Coolify | **Dinchy** |
|---|---|---|---|
| Idle memory | 350-600MB | 500-700MB | **< 200MB target** |
| API design | tRPC | REST | **REST** |
| SSO out of the box | No | Partial | **Yes** |
| Build safety | None | None | **Memory guardian** |
| Distribution | Docker | Docker | **Four small containers, no orchestrator** |
| Extend the proxy | No | No | **Yes — recompile Caddy with any plugin** |
| Tech stack | Next.js + PostgreSQL + Redis | Laravel + PostgreSQL | **Go + PostgreSQL** |

## Architecture

```
                       User traffic (80/443)
                                ↕
                     ┌─────────────────────────┐
                     │  Shared Caddy edge      │  terminates ALL TLS
                     │  - one per host         │  - auto HTTPS (ACME)
                     │  - only published port  │  - HSTS
                     └─────────────────────────┘
                                │  caddy-edge network
        /api/*  ┌───────────────┼───────────────┐  ┌─────────────────┐
                ▼               ▼               ▼  │ Edge Admin API  │
    ┌──────────────────┐  ┌──────────────┐  ┌──────┤ caddy-edge:2019 │
    │  Dinchy          │  │  Web UI      │  │ User │ (never published)│
    │  - REST API      │  │  (React SPA, │  │ apps └─────────────────┘
    │  - WebSocket     │  │   own static │  └──────┘        ▲
    │  - Terminal (PTY)│  │   container) │                  │
    └──────────────────┘  └──────────────┘   two @id-addressed objects
             │  dinchy network                            (Dinchy)
             ▼
    ┌──────────────────┐
    │ PostgreSQL/Redis │  internal only, nothing published
    └──────────────────┘
```

Single-node, and every box above is a container. **One shared Caddy edge** owns the host's
ingress and fronts every application on it — Dinchy is one tenant. Nothing else publishes a
port: the edge reaches each application over the `caddy-edge` network, and each application's
datastores sit on a network only it can see.

The edge terminates all TLS and reads nothing from disk on an application's behalf. It is
shared, so it can see none of any one application's files: the compiled UI is served by its
own small container beside the API, and the edge proxies to it like any other upstream. One
hostname is still split by path — `/api/*` reaches the Go process, everything else the UI —
which keeps the browser same-origin, so session and CSRF cookies stay `SameSite=Lax` and CORS
never comes into it.

Because the listener now faces a network rather than loopback, `DINCHY_TRUSTED_PROXIES` is
what makes the forwarded client address trustworthy: `X-Forwarded-For` is honored only from
the edge, so nothing else on that network can choose what the audit log records about it.
Dinchy refuses to start if that pair is inconsistent.

Routes live in PostgreSQL and are pushed to the edge's JSON admin API once, at startup —
**two objects, each addressed by an `@id` namespaced to this deployment**. That is what lets
several applications share one edge: a push replaces what this deployment owns and leaves
every other tenant's routes in place. Dinchy does not re-assert afterwards, so an operator
who adjusts the running proxy keeps that change, and the edge autosaves the whole result, so
a restart brings every tenant's routes back with no management plane running.

## Tech Stack

- **Backend:** Go — one static binary, no runtime dependencies
- **Database:** PostgreSQL — single supported datastore
- **Frontend:** React + Vite + Tanstack Router + Shadcn/ui — served by its own static container, never through the Go process
- **Reverse proxy:** Caddy — one shared edge per host, programmatically controlled via its JSON Admin API, automatic HTTPS, and recompilable with any Caddy plugin
- **Process supervision:** systemd via podman quadlets (Debian, Ubuntu, Fedora, Arch, Proxmox)

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

Four containers per host, driven by systemd through podman quadlets: the shared Caddy edge, the Go
API, a static server for the compiled UI, and PostgreSQL plus Redis. The edge is shared with every
other application on the host and is the only one that publishes a port. See
[deploy/README.md](deploy/README.md).

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
- **podman 4.6 or newer**, rootless. `task podman` installs it and the two things a package
  cannot supply — subordinate UID ranges and a lingering user session — and explains what it is
  doing. Below 4.6, compose health conditions are ignored with only a warning, which lets a
  migration race a cold database.

### Bootstrap

```bash
mise trust && mise install   # the pinned toolchain, and nothing else
task bootstrap               # this machine, then the stack, in the one order that works
```

Run `task bootstrap` from a terminal: installing podman and a CA root needs your password, so it
asks for one. Nothing is hung off `mise install` — installing tools is deliberately free of side
effects, which is what lets bootstrap be interactive rather than defensively silent. Every step
tests for its own result first, so running it again reports what is already true, asks for
nothing, and is the way to repair a half-finished machine.

The ordering is the whole point and is why this is a task rather than a list to follow: `.env`
before anything reads it, podman before any container, Mailpit's certificate before the stack, the
edge before `caddy trust` can read its CA, the CA trusted before the browser will accept the page,
and the application stack last. `task setup` remains the machine half alone, without the
containers.

Then edit `.env` if you want anything other than the defaults, and run `task env:check` after
pulling — it reports keys that `.env.example` has and yours does not, which is how a renamed
variable stops being a silent misconfiguration.

### The session loop

Two long-lived things, and everything that used to need its own terminal pane is a container.

| Thing | Start it with | Zed task | Lifetime |
|---|---|---|---|
| Shared Caddy edge | `task edge:up` | **Dev: start shared Caddy edge** | Outlives this project; shared with every other one |
| This project's stack | `task dev:up` | **Dev: start stack** | Postgres, Redis, Mailpit, the API under Delve, and Vite |

Then open **https://localhost:8443**. `task infra:logs` follows everything; `task dev` follows
just the backend.

The edit-and-run loop is **`task dev:watch`** (Zed: **Dev: restart API**), which restarts the
backend and recompiles from a warm build cache. There is deliberately no file watcher inside the
container: when the target process exits the debug session is over regardless, so a watcher would
only hide that.

**Ordering no longer matters.** The edge restores the routes every tenant pushed from its own
autosave, so a backend that starts first simply converges on its next attempt. Dinchy still
pushes once and does not re-assert, so changes you make to the running proxy stay yours (see
[docs/architecture.md](docs/architecture.md#one-push-at-startup-and-then-hands-off)).

Only two ports are published, both on loopback, and both are per-project knobs in `.env`:
`DINCHY_DEV_DELVE_PORT` (2345) so the debugger can attach, and `DINCHY_MAILPIT_UI_PORT` (8025)
so you can read a captured email. **That is what makes several projects workable at once** — a
second checkout moves those two numbers and stops colliding with this one. The API, Vite,
Postgres and Redis publish nothing at all; the edge reaches them by container name.

The edge terminates all TLS here exactly as it does in production, and splits one hostname by
path: `/api/*` reaches the Go process, everything else reaches Vite. Requesting `/` from the Go
process returns 404 by design.

Deployments will be reachable at **`https://<slug>.dinchy.localhost:8443`**, each with a
certificate Caddy issues on demand — nothing to regenerate when a name is added. Those names
resolve to loopback with no `/etc/hosts` entry, through `nss-myhostname` and the browsers'
built-in `.localhost` handling. Two caveats: Safari does not implement it, and Go's own
resolver bypasses NSS entirely, so a Go client needs an explicit `/etc/hosts` entry even
though curl and browsers do not.

### The shared edge

One Caddy fronts every project on your machine. It has its own compose project
(`deploy/caddy/`) and its own lifecycle, because `task infra:down` must not stop routing for
somebody else's work, and `task infra:reset` — which exists to destroy data — must not be able
to reach the volume holding the local CA your browser has been told to trust.

Two things follow from that, and both have bitten:

- **Editing `deploy/caddy/base.dev.json` does nothing to a running edge.** Caddy ignores
  `--config` whenever an autosave exists, and says so in a warning. `task edge:reset` drops the
  autosave and re-seeds; it deliberately leaves the certificate volume alone.
- **An unexpected `acme/` directory under the edge's data means a host is served by a route but
  named by no automation policy.** Uncovered hosts fall through to the default issuer, so a
  development name ends up attempting a real Let's Encrypt order.

On an SELinux host (Fedora, RHEL, Oracle Linux, Proxmox) every bind mount needs `z`, and it is
already set. Without it the container cannot read a path labelled for your home directory and the
failure names a *file* — `permission denied` on `autosave.json` — with nothing mentioning SELinux.

`deploy/caddy/README.md` has the rest, including the invariants the base configuration has to
satisfy and why the admin API's reachability is the trust boundary.

### Local configuration

**`.env` is the single source of truth**, and `.env.example` is the only committed template.
`task env:seed` copies one to the other; Task, mise and every container in `compose.yaml`
(`env_file:`) load exactly that file, so there is one place to look when a value looks wrong.
`DINCHY_LOG_FORMAT=text` is worth keeping — it produces colored, human-readable logs.

A change to `.env` now takes effect on the next **container restart** (`task dev:watch`), not on
the next debugger launch: the environment reaches the backend through compose, and Zed cannot
inject it into an already-running remote target.

Only application configuration lives here. The variables that only compose reads — which paths to
bind-mount, the two published ports — are exported by `Taskfile.yml` instead and are deliberately
absent from `.env.example`, which stays a mirror of `internal/config`.

The binary itself never reads `./.env`. `config.Load` resolves `DINCHY_ENV_FILE`, then
`~/.config/dinchy/dinchy.env`, then `/etc/dinchy/dinchy.env` (see
[`internal/config/env.go`](internal/config/env.go)), which is why `./dinchy` run by hand outside a
container gets none of your local configuration.

`DINCHY_EXPOSE_INTERNAL_ERRORS=true` adds the internal code, the cause chain (including SQL
errors) and the error metadata to every API response — the fastest way to see why a request
failed without stepping through it. It leaks internal detail by design, so it belongs in a
local `.env` only.

### Local infrastructure

Everything runs from `compose.yaml` under podman-compose, which drives podman directly — no
daemon, no socket.

**Postgres and Redis publish nothing.** They sit on a network only this project can see, and are
reached by container name, so there is no host port to collide with a system Postgres or another
checkout. The consequence is that host tooling cannot reach them either: `task db:migrate`,
`task db:status` and `task db:shell` all go through a container. `task test` is unaffected —
nothing in the suite touches a database.

**Mailpit** catches the invitation and password-reset mail so those flows work without a real
mail server, and it is configured like a production relay — STARTTLS mandatory, SMTP auth
required. Two things have to line up: the `DINCHY_SMTP_USERNAME` / `DINCHY_SMTP_PASSWORD` in your
`.env` must match `MP_SMTP_AUTH` in `compose.yaml`, or SMTP stays disabled and the invite and
reset endpoints report email as not configured; and the backend container has to trust mkcert's
root, which it does through `SSL_CERT_DIR` in `compose.yaml`. `SSL_CERT_DIR` is additive and
leaves the image's own CA bundle in play — `SSL_CERT_FILE` would replace it and break SSO and
ACME. `DINCHY_SMTP_HOST` is `mailpit` because that is one of the SANs
`task tls:mailpit-cert` issues; do not remove it.

Read the caught mail at **http://localhost:8025** — that UI is the one dev-tool port still
published, and `DINCHY_MAILPIT_UI_PORT` moves it.

A major Postgres version bump leaves behind a data directory the new version refuses to start;
`task infra:reset` is the way out, and it will ask before deleting the volumes.

> **WSL2:** if starting infra fails with `netavark: nftables error`, uncomment `NETAVARK_FW=none`
> in your `.env` — nftables is unusable on some WSL2 kernels, and rootless port forwarding still
> works without it. On native Linux, leave it unset.

> `sudo loginctl enable-linger $USER` is no longer optional, and `task podman` does it for you:
> the shared edge is meant to outlive a terminal, and without linger logging out stops it and
> takes every project's routing with it.

### Local TLS and trust

Development has two local certificate authorities because they solve different problems, and
production has neither — it uses ACME with a real domain. Caddy signs the app's certificates
with its own internal CA; mkcert issues Mailpit's, which Mailpit cannot generate for itself and
which the backend verifies against mkcert's root, mounted into its container. The reasoning is in
[docs/architecture.md](docs/architecture.md#caddy-issues-every-certificate-development-included).

`task bootstrap` installs both roots. Each goes into two independent stores: the system store,
which needs your password, and a per-user NSS database that Chrome and Firefox read *instead*
of the system one. A machine with only the system half looks correctly set up right up until
the browser rejects the page, so setup installs `certutil` and writes both. If certutil cannot
be installed on your distribution, `task tls:system` sets up the system half alone and you
accept a certificate warning in the browser.

Mailpit's certificate and key live in `$XDG_STATE_HOME/dinchy/certs` (usually
`~/.local/state/dinchy/certs`) — machine state, outside the repository, so a private key is
never in a working tree. `scripts/dev-tls.sh` holds the per-platform parts; `task tls` and its
subtasks own the ordering and the "already done" checks.

The whole `tls:*` chain is **unchanged** by the edge being a container, and that is deliberate:
the edge bind-mounts your own `$XDG_DATA_HOME/caddy`, so the CA it generates is the same CA a
host binary would read, and `caddy trust` keeps working either way.

### Extending Caddy with plugins

Caddy is a separate process rather than a library precisely so you can swap in your own build; the
reasoning is in [docs/architecture.md](docs/architecture.md#caddy-as-sidecar-not-embedded).
The baseline is [`cmd/caddy/main.go`](cmd/caddy/main.go) — upstream Caddy, no plugins, existing
only to **pin the version** in `go.mod`.

The edge image is built from that same file, which makes a blank import the natural way to add a
plugin: `go get` it, import it in `cmd/caddy/main.go`, and `task edge:image` picks it up with the
version pinned by `go.mod` like everything else.

```bash
go get github.com/caddy-dns/cloudflare      # then add the blank import to cmd/caddy/main.go
task edge:image
task caddy:modules    # confirm what actually got compiled in
```

`xcaddy` still works against the same pinned version for a throwaway experiment, writing a host
binary rather than an image:

```bash
xcaddy build "$(task caddy:version)" \
  --with github.com/caddy-dns/cloudflare \
  --output tmp/caddy
```

That last step is not bookkeeping: `caddy list-modules` is the only reliable answer to "is the
plugin loaded", because a Go module can resolve and build while registering no Caddy module at
all. The most common addition is a DNS provider, which lets Caddy answer ACME DNS-01
challenges and so issue certificates for a homelab behind NAT with no inbound port 80.

An xcaddy build is pinned only by the version you pass it — the plugin's own version is
resolved at build time and recorded nowhere, which is the reason the blank import above is the
recommended path rather than an alternative to it. This is
the one place Dinchy trades away a single-artifact install: building Caddy needs a Go toolchain
and network access.

### Debugging in Zed

[`.zed/debug.json`](.zed/debug.json) defines three configurations, named after what they debug:

| Configuration | Debugs | Needs running first |
|---------------|--------|---------------------|
| **Backend: API (Delve, in container)** | the running backend, over TCP | `task edge:up`, `task dev:up` |
| **Backend: tests in current package (Delve)** | the tests beside the file you have open | nothing |
| **Frontend: web UI (Chrome)** | `.tsx` sources in a real browser | `task edge:up`, `task dev:up` |

The backend configuration **attaches** rather than launching. The container already runs it under
a headless Delve (`--continue`, so it serves whether or not you are attached;
`--accept-multiclient`, so detaching leaves it running), and Delve refuses a launch request
against a server that already has a session — it answers *"use remote attach mode to connect to a
server with an active debug session"*. Hence `"request": "attach"` with `"mode": "remote"`.

**There is no `substitutePath`, on purpose.** `compose.yaml` mounts the repository at its own host
path, so every path Delve recorded at compile time exists on this machine and resolves as-is. If
breakpoints ever set but never bind, that mount is what changed. The same trick is what makes the
Chrome configuration work: `webRoot` points at `web/`, and Vite serves from that same host path
inside its container, so a breakpoint in a `.tsx` file binds without any mapping either.

The port is hardcoded to 2345 and has to match `DINCHY_DEV_DELVE_PORT`. Zed expands only its own
`$ZED_*` variables in `debug.json`, so a second project changes both together — that variable
exists for the *other* checkout.

There is no `envFile` any more: the environment comes from `compose.yaml`, so a change to `.env`
takes effect on the next `task dev:watch` rather than the next launch here. The test configuration
still runs on the host and deliberately loads nothing — the tests build their own environment with
`t.Setenv`, and a stray `DINCHY_*` value would mask a bug rather than reveal one.

The Chrome configuration opens `https://localhost:8443` — the edge, not Vite directly, because
only through the edge is the UI same-origin with `/api`. Chrome launches with a throwaway profile,
so no state carries between sessions.

[`.zed/tasks.json`](.zed/tasks.json) holds only the long-lived session processes and `task
check`. Everything else is `task --list` in a terminal, which cannot fall out of step with
`Taskfile.yml` the way a copy in the palette would.

> **Open this repo as the Zed project root**, not a parent folder. The debug configuration and
> the tasks resolve paths via `$ZED_WORKTREE_ROOT`, which Zed sets to the folder you opened. If
> you open an ancestor — a multi-project workspace, say — the debugger fails with `package
> cmd/dinchy is not in std`.

## License

MIT
