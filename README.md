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
| Tech stack | Next.js + PostgreSQL + Redis | Laravel + PostgreSQL | **Go + SQLite** |

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
│  - SQLite (WAL)  │ ── Docker sock ─→│  User containers   │
└──────────────────┘                  └────────────────────┘
        ↑
  systemd / OpenRC
```

Single-node, single binary. Caddy runs alongside as a sidecar and handles all routing and HTTPS.

## Tech Stack

- **Backend:** Go — single binary, no runtime dependencies
- **Database:** SQLite with WAL mode — one file, zero config, zero extra processes
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

```bash
# Run the Go backend
go run main.go

# (future) Run the frontend dev server
cd web && npm run dev
```

## License

MIT
