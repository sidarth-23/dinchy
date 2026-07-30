# The shared Caddy edge

One Caddy owns the edge on a host and fronts every application on it. Each application lives on
its own network, joins `caddy-edge`, and pushes only the slice of Caddy's configuration it owns —
one `@id`-addressed route and one certificate automation policy. Nothing but the edge publishes a
host port.

This directory is shared between development and production on purpose: the same image and the
same volume layout, differing only in ports and which base file seeds the first start.

| File | Used by |
|---|---|
| `base.dev.json` | `compose.edge.yaml`, ports 8080/8443 |
| `base.prod.json` | `../quadlet/caddy.container`, ports 80/443 |
| `compose.edge.yaml` | `task edge:up` |

## What the base configuration is for, and when it is read

It seeds the **first** start, and any start after the autosave has been removed. After that it is
never read again:

> `--resume` uses the last loaded configuration that was autosaved, overriding the `--config` flag
> (if present).

Caddy logs a warning saying it ignored `--config`, which is why passing both is safe. The
consequence is the trap: **editing a base file has no effect on a running deployment.** Use
`task edge:reset` (development) to drop the autosave and re-seed.

Every admin write autosaves the *whole* resulting document, not just the part that was written. So
one tenant's push preserves every other tenant's routes across an edge restart, with no management
plane running.

## Invariants nothing enforces

Each of these is load-bearing, and each fails in a way that does not name its cause.

- **`admin.origins` must list the address every client sends as `Host`.** With a non-loopback
  `admin.listen` and no `origins`, the only origin Caddy accepts is literally `"0.0.0.0:2019"`, so
  every admin call returns 403. Dinchy sends `Host: caddy-edge:2019`
  (`DINCHY_CADDY_ADMIN_ENDPOINT`); development also reaches it on published loopback for
  `caddy trust`.
- **`apps.http.servers.edge` must exist, with `routes` as an array.** An application writes into
  `/config/apps/http/servers/edge/routes`, and Caddy does not create missing parents — it answers
  `500 invalid traversal path at: …`. An empty `servers` object is *not* enough.
- **`apps.tls.automation.policies` must exist as an empty array**, for the same reason.
- **No catch-all automation policy.** Caddy takes the first policy whose subjects match, so a
  policy without subjects shadows every tenant's. Each tenant names its own hosts.
- **A host served by a route but named by no policy falls through to the default ACME issuer.** In
  development that is a real Let's Encrypt attempt for a hostname that cannot validate. The symptom
  is an unexpected `acme/` directory appearing under the edge's `/data`.
- **`https_port` must equal the published port.** Caddy generates port-bearing redirects from
  `https_port`, so publishing `443:8443` sends clients to the wrong port. Publish `8443:8443`.
- **Publish UDP as well as TCP on 443 in production.** HTTP/3 binds UDP; omitting it loses h3 with
  no error anywhere.

## SELinux

Every bind mount carries `z`. On an SELinux host the container otherwise cannot read a path
labelled `config_home_t` / `data_home_t`, and Caddy fails with `permission denied` on a *file* —
nothing in that message mentions SELinux. Lowercase `z` relabels to a shared label, so the host's
own `caddy` binary and `caddy trust` keep working; uppercase `Z` would make each path exclusive to
one container and break exactly that sharing.

## Why the autosave is not shared with the host

Development bind-mounts the host's `$XDG_DATA_HOME/caddy` so the local CA the edge generates is the
one `caddy trust` reads. It deliberately does **not** share the config directory: Caddy ignores
`--config` whenever an autosave exists, so a shared directory would make the edge resume whatever a
host `caddy` last wrote. On a machine that ran this project before the edge was a container, that is
a configuration naming files which no longer exist, and the edge crash-loops on startup. The
autosave is a named volume, matching what production does for both directories.

## The trust boundary

The admin API is unauthenticated. Any container that can reach it can reconfigure the entire edge,
including other applications' routes — the same property Dokploy has with Traefik. `admin.origins`
is a `Host` header check, not authentication.

So reachability is the boundary, and it is deliberately narrow: `2019` is **never published** in
production, which keeps it inside the host. An operator reaches it with
`podman exec caddy-edge wget -qO- localhost:2019/config/`. Development publishes it on `127.0.0.1`
only, because `caddy trust` and `scripts/dev-tls.sh` read the local CA through it.

Every container on `caddy-edge` is therefore fully trusted. Caddy's `admin.identity` with a
`remote` admin endpoint is the way to change that when untrusted tenants appear.
