# Deploying Dinchy

Dinchy runs as **four rootless podman containers** on one host, declared as podman [Quadlet] units
and supervised by systemd. A single env file (`/etc/dinchy/dinchy.env`) configures the app and its
datastores.

The units are `.container` files; podman + systemd generate the matching `.service` units
automatically. They are podman-managed services, not units you maintain by hand.

> **The shared Caddy edge is not Dinchy's.** One edge fronts every application on the host, and
> Dinchy is one tenant on it — it writes only the two configuration objects it owns. If the host
> already runs an edge, do not install a second one; point `DINCHY_CADDY_ADMIN_ENDPOINT` at the
> existing one and skip that unit.

> `deploy/caddy/` is shared with local development on purpose: the same image, the same volume
> layout, differing only in ports and which base file seeds the first start. Everything else in
> this directory is production-only — see the root [README](../README.md#development) for the
> development path.

```
                        User traffic (80/443, 443/udp)
                                  ↕
        ┌──────────────────────────────────────────────────────┐
        │  caddy.service  (caddy-edge)                          │  terminates ALL TLS
        │   the only published port on the host                  │  auto HTTPS via ACME
        │   admin API on :2019 — never published                 │  network: caddy-edge
        └──────────────────────────────────────────────────────┘
              │ dinchy-api:8080          │ dinchy-web-static:8081
              ▼                          ▼
        ┌──────────────────────┐   ┌──────────────────────┐
        │  dinchy.service      │   │  dinchy-web.service  │  the compiled SPA,
        │  (dinchy-api)        │   │  (dinchy-web-static) │  its own static server
        │   two @id objects ──►│   └──────────────────────┘
        │   to the edge's API  │
        └──────────────────────┘
              │ dinchy-postgres:5432 / dinchy-redis:6379
              ▼
        ┌──────────────────────────────────────┐
        │  postgres.service   redis.service     │  network: dinchy
        │  (dinchy-postgres)  (dinchy-redis)    │  nothing published
        └──────────────────────────────────────┘
              dinchy.service and the datastores read /etc/dinchy/dinchy.env
```

**Nothing but the edge publishes a port.** Every other container is reachable only on a podman
network: `caddy-edge`, which the edge shares with every application, or `dinchy`, which carries
this application's datastores and which no other tenant joins.

Dinchy never terminates TLS. It listens plaintext and configures the edge through Caddy's JSON
admin API; there is no Caddyfile to install or maintain. Because its listener faces a network
rather than loopback, `DINCHY_TRUSTED_PROXIES` is what keeps the forwarded client address in audit
rows trustworthy, and Dinchy refuses to start if that pair is inconsistent.

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
| `dinchy.env.example` | `/etc/dinchy/dinchy.env` | Shared config for the app + its datastores |
| `caddy/base.prod.json` | `/etc/dinchy/caddy-base.json` | Seeds the edge's first start |
| `caddy/README.md` | (stays in the repo) | The invariants that base configuration must satisfy |
| `../cmd/caddy/main.go` | (stays in the repo) | The vanilla Caddy build; pins the version |
| `quadlet/caddy-edge.network` | `~/.config/containers/systemd/` | Podman network `caddy-edge`, shared by every app |
| `quadlet/caddy.container` | `~/.config/containers/systemd/` | The shared Caddy edge |
| `quadlet/dinchy.network` | `~/.config/containers/systemd/` | Podman network `dinchy` for the datastores |
| `quadlet/dinchy.container` | `~/.config/containers/systemd/` | The Dinchy API |
| `quadlet/dinchy-web.container` | `~/.config/containers/systemd/` | The compiled web UI |
| `quadlet/postgres.container` | `~/.config/containers/systemd/` | PostgreSQL container |
| `quadlet/redis.container` | `~/.config/containers/systemd/` | Redis container |

Quadlet derives a unit name from the filename, so `caddy.container` generates `caddy.service` —
which is exactly what `dinchy.container`'s `After=`/`Wants=caddy.service` refers to. Renaming that
file silently breaks the ordering; don't.

## Install (rootless, recommended)

```sh
# 1. Dedicated user, with lingering so its services run without an active login.
sudo useradd --system --create-home --shell /usr/sbin/nologin dinchy
sudo loginctl enable-linger dinchy

# 2. Images. Built where a Go toolchain and network access are available, not on this host.
#    `task images` produces all three; move them across with `podman save` / `podman load`,
#    or push to a registry the host can pull from.
task images                                          # on the build machine
podman save localhost/dinchy-caddy:latest localhost/dinchy:latest localhost/dinchy-web:latest \
  | ssh HOST 'sudo -u dinchy podman load'
# Let the edge bind 80/443 rootless. Still needed with a container: the rootless port
# forwarder runs unprivileged on the host side.
echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee /etc/sysctl.d/50-dinchy-caddy.conf
sudo sysctl --system

# 3. Config. Fill in secrets, and keep the Postgres password in sync between
#    DINCHY_POSTGRES_DSN and POSTGRES_PASSWORD.
sudo install -d -m 0750 -o dinchy -g dinchy /etc/dinchy
sudo install -m 0640 -o dinchy -g dinchy dinchy.env.example /etc/dinchy/dinchy.env
sudo install -m 0644 -o dinchy -g dinchy caddy/base.prod.json /etc/dinchy/caddy-base.json
sudoedit /etc/dinchy/dinchy.env

# 4. Units (as the dinchy user).
sudo -u dinchy install -d ~dinchy/.config/containers/systemd
sudo -u dinchy install -m 0644 quadlet/*.container quadlet/*.network ~dinchy/.config/containers/systemd/

# 5. Start the datastores, migrate, then start the rest. The units order themselves, so
#    `enable --now dinchy` alone also works — the edge is pulled in first.
export AS_DINCHY="sudo -u dinchy XDG_RUNTIME_DIR=/run/user/$(id -u dinchy)"
$AS_DINCHY systemctl --user daemon-reload
$AS_DINCHY systemctl --user start postgres redis
# Migrations run in a container, because Postgres publishes no port for a host client to use.
$AS_DINCHY podman run --rm --network dinchy --env-file /etc/dinchy/dinchy.env \
  localhost/dinchy:latest --migrate-only    # or exec goose in a throwaway container
$AS_DINCHY systemctl --user enable --now caddy
$AS_DINCHY systemctl --user enable --now dinchy-web
$AS_DINCHY systemctl --user enable --now dinchy
```

## Verify

```sh
systemctl --user status caddy dinchy dinchy-web postgres redis
# Only the edge listens on the host. Postgres, Redis, the API and the UI must NOT appear here.
ss -ltn | grep -E ':(80|443)\b'
# The admin API is not published, so it is reached from inside the container.
podman exec caddy-edge wget -qO- http://127.0.0.1:2019/config/ | head -c 400
# This deployment's two objects, by their @id.
podman exec caddy-edge wget -qO- http://127.0.0.1:2019/id/dinchy.dinchy.routes
podman exec dinchy-api wget -qO- http://127.0.0.1:9090/readyz    # internal health
curl -fsS https://<panel-host>/api/auth/sso/providers            # through the edge, real certificate
```

## Requirements

- **podman 4.6+**, rootless, with lingering enabled for the service user. The edge is meant to
  outlive any login session.
- **Redis 8.4+.** The event bus consumes with `XREADGROUP ... CLAIM` (single-call reclaim of
  idle pending entries), an option introduced in Redis 8.4. Redis 8.0–8.3, Valkey, and
  DragonflyDB do not implement it, so the quadlet pins `redis:8.4`.
- **A Go toolchain and network access**, but only to *build the images*. The host that runs them
  needs neither.

## Security boundaries

These are constraints, not conventions: each one is load-bearing, and the deployment is not
secure without it.

- **`DINCHY_TRUSTED_PROXIES` must name the edge's network, and Dinchy refuses to start
  otherwise.** The app has no transport check of its own and records the forwarded client address
  in `audit_logs.ip_address` and `sessions.ip_address`. Its listener faces `caddy-edge` rather than
  loopback, so anything else on that network could reach it directly — the trusted-proxy set is
  what stops such a peer choosing what the audit log records about it. A listener reachable beyond
  loopback with only loopback trusted is rejected at startup: it is not forgeable, but it would
  record the edge's own address for every request, which is wrong in a way nobody would notice.
- **Nothing but the edge publishes a port.** That is the containment, and it is why the API, the
  UI, Postgres and Redis are absent from `ss -ltn`. A `PublishPort=` added to any of those units
  removes it.
- **The edge's admin API is unauthenticated, and every container on `caddy-edge` is therefore
  fully trusted.** Anything that can reach `DINCHY_CADDY_ADMIN_ENDPOINT` can load an arbitrary
  configuration and so reconfigure the whole edge — including *other applications'* routes. This
  is the same trade Dokploy makes with Traefik. Two consequences worth stating plainly: the admin
  port is deliberately never published, so the boundary is the host; and `admin.origins` in the
  base configuration is a `Host` header check, not authentication, so it is not the boundary.
  Caddy's `admin.identity` with a `remote` admin endpoint is how to change that if untrusted
  tenants ever join the network. **Do not attach a user deployment's container to `caddy-edge`
  unless it is trusted to that degree.**
- **Dinchy holds the container socket.** `dinchy.container` mounts it because the management plane
  manages user deployments through it. It is root-equivalent on this host, and it is the most
  consequential grant in the deployment.
- **Redis runs unauthenticated on loopback.** If you expose it or want auth, add a password
  via the container command and set `DINCHY_REDIS_PASSWORD`.
- **The single env file is mounted into the Postgres container**, so SMTP and SSO secrets are
  present in that container's environment. Acceptable for a trusted single host; split the
  file if you need stricter isolation. The edge deliberately gets no `EnvironmentFile` at all —
  it reads no `DINCHY_*` variable, so giving it the secrets would be pure exposure.

## Operating

- **The edge needs a base configuration once, and then never again.** It runs as
  `caddy run --resume --config /etc/caddy/base.json`. Caddy ignores `--config` whenever an
  autosave exists — logging a warning saying so — which means the base file seeds the *first*
  start and any start after the `caddy-edge-config` volume has been emptied, and nothing else.
  **Editing `/etc/dinchy/caddy-base.json` has no effect on a running edge.** To re-seed it, stop
  the edge, `podman volume rm caddy-edge-config`, and start it again — and do not touch
  `caddy-edge-data`, which holds the certificates.

  The base file is not a formality. Caddy refuses to traverse into a path whose parents are
  absent, so the HTTP server named by `DINCHY_CADDY_EDGE_SERVER` and the automation policies array
  both have to exist before any tenant can address anything inside them. `caddy/README.md` lists
  every invariant and what breaks when one is missing.

- **Every admin write autosaves the whole resulting configuration**, not just the part written. So
  one application's push preserves every other tenant's routes across an edge restart, with no
  management plane running at all. That is what makes restart ordering a convenience rather than a
  requirement: an application that starts before the edge simply converges on its next attempt.

- **Dinchy does not re-assert the configuration.** There is no drift job. Once the routes
  are pushed, changes you make to the running proxy stay, and Dinchy will not overwrite them
  on a timer. The cost is that a push which fails at startup is not retried later — check
  `journalctl --user -u dinchy` for "Caddy reconcile at startup failed" and restart Dinchy
  after fixing whatever was wrong.

- **The edge serves no files.** It fronts every application on the host and can read none of
  their files, so the compiled UI ships as its own container (`dinchy-web.container`) and the edge
  proxies to it like any other upstream — `DINCHY_FRONTEND_URL` names it. The single-page fallback
  to `index.html` lives in that image, beside the files it falls back to. One hostname is still
  split by path: `/api/*` to Dinchy, everything else to the UI. Sharing one hostname is
  deliberate — it keeps the browser same-origin, which is what lets the session and CSRF cookies
  stay `SameSite=Lax`.

- **Two volumes, and they are not interchangeable.** `caddy-edge-data` (`/data`) holds the local
  CA, ACME account keys and every issued certificate; losing it re-registers with the certificate
  authority and re-issues everything, which is a fast way to hit Let's Encrypt's per-domain rate
  limit. `caddy-edge-config` (`/config`) holds only the autosave — the configuration every tenant
  pushed; losing it means no routing until each management plane restarts and pushes again. The
  image sets `XDG_DATA_HOME` and `XDG_CONFIG_HOME` so both land in those volumes and an operator
  cannot silently end up with a different layout. Back `caddy-edge-data` up with the database.

- **An unexpected `acme/` directory under `caddy-edge-data` is a symptom.** It means a host is
  served by a route but named by no automation policy, so it fell through to the default issuer.

- **Redis data persists across restarts.** The quadlet mounts a named volume at `/data`,
  which holds Redis persistence files.

- **Rebuilding the edge with plugins.** Add a blank import to `cmd/caddy/main.go`, `go get` it so
  `go.mod` pins it, `task images`, load the new image, and `systemctl --user restart caddy`. The
  restart is safe because `--resume` restores every tenant's routes and no management plane need be
  running. Confirm the plugin registered with
  `podman run --rm localhost/dinchy-caddy:latest list-modules`: a Go module can resolve and build
  while registering no Caddy module at all.

- **Datastore TLS.** `dinchy.env.example` connects to Postgres with `sslmode=verify-full`
  and Redis with `DINCHY_REDIS_TLS=true`. Provision loopback server certificates for the
  Postgres/Redis containers, mount them into the quadlets, enable TLS in each container,
  and point `sslrootcert` at the signing CA. The server certificates must carry
  `dinchy-postgres` / `dinchy-redis` as SANs rather than an IP: the DSN and
  `DINCHY_REDIS_ADDR` name containers now, not loopback. To run the datastores without TLS, drop
  `?sslmode=verify-full&sslrootcert=...` from the DSN and unset `DINCHY_REDIS_TLS`.

- **Rootful alternative:** place the quadlets in `/etc/containers/systemd/`, swap
  `systemctl --user` for `systemctl`, and set `WantedBy=multi-user.target`. Note that the
  container socket `dinchy.container` mounts is then the rootful one.

## Troubleshooting

- **If routing breaks, the panel is still reachable.** The edge's failure never stops Dinchy —
  `dinchy.container` declares `Wants=caddy.service`, not `Requires=` — and the API keeps serving on
  its own listener inside the container network. Reach it over SSH and fix the routing from the UI:

  ```sh
  # Forward the API out of the container network, without publishing anything permanently.
  ssh -L 8080:127.0.0.1:8080 dinchy@host \
    'podman run --rm --network caddy-edge -p 127.0.0.1:8080:8080 alpine/socat \
       TCP-LISTEN:8080,fork TCP:dinchy-api:8080'
  ```

  No special headers are needed — the app does not reject plaintext. Readiness probes the edge's
  admin API on each request and reports it as `degraded` rather than failing, so an orchestrator
  will not restart the management plane over a routing fault.

- **The edge cannot bind `:80`/`:443`.** Rootless port forwarding runs unprivileged on the host
  side, so this is still needed with a container: `sudo sysctl -w net.ipv4.ip_unprivileged_port_start=80`,
  persisted in `/etc/sysctl.d/`. Otherwise publish high ports behind your firewall and change
  `https_port`/`http_port` in `/etc/dinchy/caddy-base.json` to match — Caddy generates
  port-bearing redirects from `https_port`, so publishing `443:8443` would send clients to the
  wrong port.

- **HTTP/3 stopped working.** `PublishPort=443:443/udp` is a separate line from the TCP one, and
  omitting it loses h3 with no error anywhere.

- **Every admin call returns 403.** `admin.origins` in the base configuration has to list the
  address clients send as `Host`. With a non-loopback `admin.listen` and no `origins`, the only
  origin Caddy accepts is literally `0.0.0.0:2019`, so `DINCHY_CADDY_ADMIN_ENDPOINT` must appear
  there.

- **A push fails with `invalid traversal path`.** The edge's base configuration is missing the
  server or the policies array this deployment writes into. Dinchy reports it separately from a
  rejected route for exactly this reason: the fix is in `/etc/dinchy/caddy-base.json`, not in the
  application.

- **A route is served but has no certificate.** Check that the host appears in this deployment's
  automation policy (`podman exec caddy-edge wget -qO- http://127.0.0.1:2019/id/dinchy.dinchy.tls`).
  A host covered by no policy falls through to the default issuer.

[Quadlet]: https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html
