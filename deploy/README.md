# Deploying Dinchy

Dinchy runs as a **single host-native systemd service**. Its infrastructure
dependencies — PostgreSQL and Redis — run as **rootless podman containers** in a
dedicated `dinchy` user namespace, declared as podman [Quadlet] units. A single
env file (`/etc/dinchy/dinchy.env`) configures the app *and* the containers.

You hand-author exactly one unit — `dinchy.service`. The Postgres and Redis
containers are declared as `.container` files; podman + systemd generate their
`postgres.service` / `redis.service` units automatically. They are
podman-managed services, not units you maintain.

> **Local development** uses a different, lighter path: `compose.yaml` at the repo root,
> run with `mise run infra:up` (podman-compose, driving podman directly — no Docker). The
> quadlet units here are the production model; the dev compose file is not used in production.

```
                 systemd (dinchy user session)
        ┌───────────────────────────────────────────┐
        │  dinchy.service ── /usr/local/bin/dinchy    │  host-native binary
        │        │  connects on 127.0.0.1:5432/6379   │
        │        ▼                                    │
        │  postgres.service   redis.service           │  podman quadlets
        │  (dinchy-postgres)  (dinchy-redis)          │  network: dinchy
        └───────────────────────────────────────────┘
                 all read /etc/dinchy/dinchy.env
```

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
- **Secrets are visible to the Postgres container.** Because the single file is
  mounted into it, SMTP/SSO secrets are present in that container's environment.
  This is acceptable for a trusted single-host deployment; split the file if you
  need stricter isolation.

## Files

| File | Installs to | Purpose |
|------|-------------|---------|
| `dinchy.env.example` | `/etc/dinchy/dinchy.env` | Shared config for app + containers |
| `systemd/dinchy.service` | `~/.config/systemd/user/` | The Dinchy server unit |
| `quadlet/dinchy.network` | `~/.config/containers/systemd/` | Podman network `dinchy` |
| `quadlet/postgres.container` | `~/.config/containers/systemd/` | PostgreSQL container |
| `quadlet/redis.container` | `~/.config/containers/systemd/` | Redis container |

## Install (rootless, recommended)

```sh
# 1. Dedicated user, with lingering so its services run without an active login.
sudo useradd --system --create-home --shell /usr/sbin/nologin dinchy
sudo loginctl enable-linger dinchy

# 2. Binary.
sudo install -m 0755 dinchy /usr/local/bin/dinchy

# 3. Config. Fill in secrets, and keep the Postgres password in sync between
#    DINCHY_POSTGRES_DSN and POSTGRES_PASSWORD.
sudo install -d -m 0750 -o dinchy -g dinchy /etc/dinchy
sudo install -m 0640 -o dinchy -g dinchy dinchy.env.example /etc/dinchy/dinchy.env
sudoedit /etc/dinchy/dinchy.env

# 4. Units (as the dinchy user).
sudo -u dinchy install -d ~dinchy/.config/containers/systemd ~dinchy/.config/systemd/user
sudo -u dinchy install -m 0644 quadlet/*.container quadlet/*.network ~dinchy/.config/containers/systemd/
sudo -u dinchy install -m 0644 systemd/dinchy.service ~dinchy/.config/systemd/user/

# 5. Start infra, migrate, then start the app.
sudo -u dinchy XDG_RUNTIME_DIR=/run/user/$(id -u dinchy) systemctl --user daemon-reload
sudo -u dinchy XDG_RUNTIME_DIR=/run/user/$(id -u dinchy) systemctl --user start postgres redis
DINCHY_ENV_FILE=/etc/dinchy/dinchy.env mise run db:migrate   # or: goose ... up
sudo -u dinchy XDG_RUNTIME_DIR=/run/user/$(id -u dinchy) systemctl --user enable --now dinchy
```

## Verify

```sh
systemctl --user status postgres redis dinchy
ss -ltn | grep -E '127.0.0.1:(5432|6379)'        # infra on loopback
curl -fsS http://127.0.0.1:9090/                 # internal health endpoint (loopback, plaintext)
curl -fsSk https://127.0.0.1/auth/sso/providers  # enabled SSO providers, over the app's HTTPS listener
```

## Notes

- **Redis 8.4+ is required.** The event bus consumes with `XREADGROUP ... CLAIM`
  (single-call reclaim of idle pending entries), an option introduced in Redis 8.4.
  Redis 8.0–8.3, Valkey, and DragonflyDB do not implement it, so the quadlet pins
  `redis:8.4`.
- **Redis data persists across restarts.** The quadlet mounts a named volume at
  `/data`, which holds Redis persistence files.
- **Rootful alternative:** place the quadlets in `/etc/containers/systemd/` and
  `dinchy.service` in `/etc/systemd/system/` (with `User=dinchy`), swap
  `systemctl --user` for `systemctl`, and set `WantedBy=multi-user.target`.
- **Redis** runs unauthenticated on loopback. If you expose it or want auth, add a
  password via the container command and set `DINCHY_REDIS_PASSWORD`.
- **TLS.** The Go server terminates HTTPS itself from `DINCHY_TLS_CERT_FILE` /
  `DINCHY_TLS_KEY_FILE` (operator-provided — e.g. certbot or your load balancer's cert).
  Binding `DINCHY_ADDR=:443` from the rootless `--user` unit needs the capability —
  add `AmbientCapabilities=CAP_NET_BIND_SERVICE` (running the unit system-level),
  `sudo setcap cap_net_bind_service=+ep /usr/local/bin/dinchy`, or lower
  `net.ipv4.ip_unprivileged_port_start`; otherwise use a high port such as `:8443`
  behind your firewall/port-forward. Leaving both cert vars empty serves plain HTTP on
  loopback for an external TLS-terminating proxy that sets `X-Forwarded-Proto`.
  Automatic Let's Encrypt is intentionally deferred to a future Caddy integration.
- **Datastore TLS.** `dinchy.env.example` connects to Postgres with `sslmode=verify-full`
  and Redis with `DINCHY_REDIS_TLS=true`. Provision loopback server certificates for the
  Postgres/Redis containers, mount them into the quadlets, enable TLS in each container,
  and point `sslrootcert` at the signing CA. To run the datastores without TLS, drop
  `?sslmode=verify-full&sslrootcert=...` from the DSN and unset `DINCHY_REDIS_TLS`.

[Quadlet]: https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html
