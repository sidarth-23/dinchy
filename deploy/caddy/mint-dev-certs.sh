#!/usr/bin/env bash
# Mint local TLS certs for the dev Postgres/Redis/Mailpit containers, signed by
# Caddy's internal CA, so the app talks to all infra over verified TLS — matching
# production (no dev-only plaintext bypass).
#
# Prerequisite: Caddy must have generated its internal CA. Start it once
# (`mise run caddy`, then `caddy trust`) before running this. The CA root lives at
# $XDG_DATA_HOME/caddy (or ~/.local/share/caddy) under pki/authorities/local.
set -euo pipefail

CERT_DIR="deploy/caddy/dev-certs"
CADDY_DATA="${XDG_DATA_HOME:-$HOME/.local/share}/caddy"
CA_ROOT="$CADDY_DATA/pki/authorities/local/root.crt"
CA_KEY="$CADDY_DATA/pki/authorities/local/root.key"

if [[ ! -f "$CA_ROOT" || ! -f "$CA_KEY" ]]; then
	echo "Caddy internal CA not found at $CADDY_DATA/pki/authorities/local/." >&2
	echo "Start Caddy once and trust it first:  mise run caddy  (then, in another shell)  caddy trust" >&2
	exit 1
fi

mkdir -p "$CERT_DIR"
cp "$CA_ROOT" "$CERT_DIR/ca.crt"

# One leaf per service; all reachable on loopback, so SANs cover localhost + 127.0.0.1
# plus the compose service name (used inside the podman network).
for svc in postgres redis mailpit; do
	step certificate create "$svc" \
		"$CERT_DIR/$svc.crt" "$CERT_DIR/$svc.key" \
		--ca "$CA_ROOT" --ca-key "$CA_KEY" \
		--san "$svc" --san localhost --san 127.0.0.1 \
		--not-after 8760h --no-password --insecure --force
	# Postgres refuses a key readable by group/other; keep leaves owner-only.
	chmod 600 "$CERT_DIR/$svc.key"
done

echo "Wrote CA + service certs to $CERT_DIR (trusted via Caddy's internal CA)."
