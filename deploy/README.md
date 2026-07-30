# Deploying Dinchy

Dinchy runs as a **single host-native systemd service**. Its infrastructure
dependencies — PostgreSQL and Redis — run as **rootless podman containers** in a
dedicated `dinchy` user namespace, declared as podman [Quadlet] units. A single
env file (`/etc/dinchy/dinchy.env`) configures the app *and* the containers.

You hand-author exactly one unit — `dinchy.service`. The Postgres and Redis
containers are declared as `.container` files; podman + systemd generate their
`postgres.service` / `redis.service` units automatically. They are
podman-managed services, not units you maintain.

> **Local development** uses a different, lighter path — see the root
> [README](../README.md#development). The quadlet units here are the production model and are
> not used in development; nothing in this directory is a development artifact.

```
                        User traffic (80/443)
                                  ↕
                 systemd (dinchy user session)
        ┌─────────────────────────────────────────────┐
        │  caddy.service  ── /usr/local/bin/caddy      │  terminates ALL TLS
        │        │  proxies to 127.0.0.1:8080          │  auto HTTPS via ACME
        │        ▼                                     │
        │  dinchy.service ── /usr/local/bin/dinchy     │  host-native, plaintext
        │        │  pushes routes to 127.0.0.1:2019    │  loopback only
        │        │  connects on 127.0.0.1:5432/6379    │
        │        ▼                                     │
        │  postgres.service   redis.service            │  podman quadlets
        │  (dinchy-postgres)  (dinchy-redis)           │  network: dinchy
        └─────────────────────────────────────────────┘
                 all read /etc/dinchy/dinchy.env
```

Dinchy never terminates TLS. It listens plaintext on loopback and configures Caddy through
Caddy's JSON admin API; there is no Caddyfile to install or maintain.

## Configuration

`dinchy.env.example` documents **every** supported variable with a one-line
comment and its default (or empty for optional/secret values). Copy it to
`/etc/dinchy/dinchy.env`, then:

- **One file, two consumers.** The `DINCHY_*` keys configure the app; the
  `POSTGRES_*` keys configure the Postgres container. Both `dinchy.service` and
  the quadlets load this file via `EnvironmentFile=`. Postgres ignores the
  `DINCHY_*` keys it doesn't recognise.
- **Load order** (first match wins), resolved by the app itself:
  `DINCHY_ENV_FILE` → `$XDG_CONFIG_HOME/dinchy/dinchy.env` (default
  `~/.config/dinchy/dinchy.env`) → `/etc/dinchy/dinchy.env`. Variables already
  present in the process environment are **not** overridden, so systemd
  `Environment=` entries win over the file.
- **No `${...}` expansion.** systemd `EnvironmentFile` does not interpolate
  variables, so the Postgres password appears literally in both
  `DINCHY_POSTGRES_DSN` and `POSTGRES_PASSWORD` — keep the two in sync.
- **Secrets are visible to the Postgres container**, because the single file is mounted into
  it — see "Security boundaries" below.

## Files

| File | Installs to | Purpose |
|------|-------------|---------|
| `dinchy.env.example` | `/etc/dinchy/dinchy.env` | Shared config for app + containers |
| `systemd/dinchy.service` | `~/.config/systemd/user/` | The Dinchy server unit |
| `systemd/caddy.service` | `~/.config/systemd/user/` | The Caddy reverse proxy unit |
| `../cmd/caddy/main.go` | (stays in the repo) | The vanilla Caddy build; pins the version |
| `quadlet/dinchy.network` | `~/.config/containers/systemd/` | Podman network `dinchy` |
| `quadlet/postgres.container` | `~/.config/containers/systemd/` | PostgreSQL container |
| `quadlet/redis.container` | `~/.config/containers/systemd/` | Redis container |

## Install (rootless, recommended)

```sh
# 1. Dedicated user, with lingering so its services run without an active login.
sudo useradd --system --create-home --shell /usr/sbin/nologin dinchy
sudo loginctl enable-linger dinchy

# 2. Binaries. Caddy comes from `task caddy:build` (vanilla, version pinned by go.mod),
#    or from `xcaddy build "$(task caddy:version)" --with <module>` if you want plugins.
sudo install -m 0755 dinchy /usr/local/bin/dinchy
sudo install -m 0755 caddy /usr/local/bin/caddy
# Let Caddy bind 80/443 without root (survives rebuilding the binary).
echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee /etc/sysctl.d/50-dinchy-caddy.conf
sudo sysctl --system

# 3. Config. Fill in secrets, and keep the Postgres password in sync between
#    DINCHY_POSTGRES_DSN and POSTGRES_PASSWORD.
sudo install -d -m 0750 -o dinchy -g dinchy /etc/dinchy
sudo install -m 0640 -o dinchy -g dinchy dinchy.env.example /etc/dinchy/dinchy.env
sudoedit /etc/dinchy/dinchy.env

# 4. Units (as the dinchy user).
sudo -u dinchy install -d ~dinchy/.config/containers/systemd ~dinchy/.config/systemd/user
sudo -u dinchy install -m 0644 quadlet/*.container quadlet/*.network ~dinchy/.config/containers/systemd/
sudo -u dinchy install -m 0644 systemd/dinchy.service systemd/caddy.service ~dinchy/.config/systemd/user/

# 5. Start infra, migrate, then start the app.
sudo -u dinchy XDG_RUNTIME_DIR=/run/user/$(id -u dinchy) systemctl --user daemon-reload
sudo -u dinchy XDG_RUNTIME_DIR=/run/user/$(id -u dinchy) systemctl --user start postgres redis
DINCHY_ENV_FILE=/etc/dinchy/dinchy.env task db:migrate   # or: goose ... up
# Caddy first: Dinchy pushes it the routes once, at its own startup. The units already
# order themselves this way, so `enable --now dinchy` alone would also work.
sudo -u dinchy XDG_RUNTIME_DIR=/run/user/$(id -u dinchy) systemctl --user enable --now caddy
sudo -u dinchy XDG_RUNTIME_DIR=/run/user/$(id -u dinchy) systemctl --user enable --now dinchy
```

## Verify

```sh
systemctl --user status postgres redis dinchy caddy
ss -ltn | grep -E '127.0.0.1:(5432|6379)'        # infra on loopback
curl -fsS http://127.0.0.1:9090/readyz          # internal health; caddy reported as ok/degraded
curl -fsS localhost:2019/config/                # the configuration Dinchy pushed to Caddy
curl -fsS https://<panel-host>/api/auth/sso/providers   # through Caddy, with a real certificate
curl -fsS http://127.0.0.1:8080/api/bootstrap   # Dinchy directly, bypassing Caddy
```

## Requirements

- **Redis 8.4+.** The event bus consumes with `XREADGROUP ... CLAIM` (single-call reclaim of
  idle pending entries), an option introduced in Redis 8.4. Redis 8.0–8.3, Valkey, and
  DragonflyDB do not implement it, so the quadlet pins `redis:8.4`.
- **A Go toolchain and network access**, but only to build the two binaries. The host that
  runs them needs neither.

## Security boundaries

These are constraints, not conventions: each one is load-bearing, and the deployment is not
secure without it.

- **`DINCHY_ADDR` must stay on loopback, and Dinchy refuses to start otherwise.** The app has
  no transport check of its own and trusts the forwarded client address, so restricting who
  can reach the listener is what replaces one. Caddy is the only TLS terminator and the only
  ingress; Dinchy serves plaintext on `127.0.0.1:8080` in every environment.
- **The admin API is unauthenticated.** Anything that can reach
  `DINCHY_CADDY_ADMIN_ENDPOINT` can load an arbitrary configuration and so reconfigure the
  proxy. Keep it on loopback, and remember that deployment containers run on this host.
- **Redis runs unauthenticated on loopback.** If you expose it or want auth, add a password
  via the container command and set `DINCHY_REDIS_PASSWORD`.
- **The single env file is mounted into the Postgres container**, so SMTP and SSO secrets are
  present in that container's environment. Acceptable for a trusted single host; split the
  file if you need stricter isolation.

## Operating

- **Caddy needs no config file.** It runs as `caddy run --resume`: Dinchy pushes the routes
  over the admin API, and Caddy restores the last pushed configuration by itself across
  restarts. Caddy starts first (`dinchy.service` orders itself `After=caddy.service`), and
  Dinchy converges it once at its own startup, waiting a few seconds if Caddy's admin
  endpoint is not answering yet.

- **Dinchy does not re-assert the configuration.** There is no drift job. Once the routes
  are pushed, changes you make to the running proxy stay, and Dinchy will not overwrite them
  on a timer. The cost is that a push which fails at startup is not retried later — check
  `journalctl --user -u dinchy` for "Caddy reconcile at startup failed" and restart Dinchy
  after fixing whatever was wrong.

- **Caddy serves the web UI itself.** One hostname is split by path: `/api/*` is proxied to
  Dinchy and everything else is served from `DINCHY_FRONTEND_ROOT` (default `web/dist`) as
  static files, with unmatched paths falling back to `index.html` so client-side routes
  survive a reload. Install the compiled assets there and make sure the Caddy unit's user
  can read them. Sharing one hostname is deliberate: it keeps the browser same-origin, which
  is what lets the session and CSRF cookies stay `SameSite=Lax`.

- **Certificate storage must be persistent.** `caddy.service` pins `XDG_DATA_HOME` so
  certificates and ACME account keys live at a stable path. If that path moves, Caddy
  re-registers with the certificate authority and re-issues every certificate, which is a
  fast way to hit Let's Encrypt's per-domain rate limit. Back it up with the database.

- **Redis data persists across restarts.** The quadlet mounts a named volume at `/data`,
  which holds Redis persistence files.

- **Rebuilding Caddy with plugins.** `xcaddy build "$(task caddy:version)" --with <module>`,
  install the result to `/usr/local/bin/caddy`, and `systemctl --user restart caddy`. Passing
  the pinned version keeps your build on the same Caddy as the baseline. The restart is safe
  because `--resume` restores the routes, and Dinchy need not be running. Confirm the plugin
  registered with `caddy list-modules`: a Go module can resolve and build while registering
  no Caddy module at all.

- **Datastore TLS.** `dinchy.env.example` connects to Postgres with `sslmode=verify-full`
  and Redis with `DINCHY_REDIS_TLS=true`. Provision loopback server certificates for the
  Postgres/Redis containers, mount them into the quadlets, enable TLS in each container,
  and point `sslrootcert` at the signing CA. To run the datastores without TLS, drop
  `?sslmode=verify-full&sslrootcert=...` from the DSN and unset `DINCHY_REDIS_TLS`.

- **Rootful alternative:** place the quadlets in `/etc/containers/systemd/` and
  `dinchy.service` in `/etc/systemd/system/` (with `User=dinchy`), swap
  `systemctl --user` for `systemctl`, and set `WantedBy=multi-user.target`.

## Troubleshooting

- **If routing breaks, the panel is still reachable.** Caddy's failure never stops Dinchy —
  `dinchy.service` declares `Wants=caddy.service`, not `Requires=` — and Dinchy keeps
  serving on `127.0.0.1:8080`. Reach it over SSH and fix the routing from the UI:

  ```sh
  ssh -L 8080:127.0.0.1:8080 dinchy@host
  curl http://127.0.0.1:8080/api/bootstrap    # then browse http://127.0.0.1:8080
  ```

  No special headers are needed — the app does not reject plaintext, because the loopback
  bind is what keeps the network out. Readiness probes Caddy's admin API on each request
  and reports it as `degraded` rather than failing, so an orchestrator will not restart the
  management plane over a routing fault.

- **Caddy cannot bind `:80`/`:443`.** A rootless `--user` unit needs the capability. Prefer
  `sudo sysctl -w net.ipv4.ip_unprivileged_port_start=80`, persisted in `/etc/sysctl.d/`,
  which survives rebuilding the Caddy binary. `sudo setcap cap_net_bind_service=+ep
  /usr/local/bin/caddy` also works but is **lost every time the binary is replaced**, so it
  has to be reapplied after every rebuild — which is the reason to prefer the sysctl.
  Otherwise use a high `DINCHY_CADDY_HTTPS_PORT` behind your firewall, or port-forward.
  `AmbientCapabilities=` is ignored in a systemd *user* unit, which is why it is not used
  here; both units are user units so they can be ordered against each other and against the
  rootless podman quadlets.

[Quadlet]: https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html
