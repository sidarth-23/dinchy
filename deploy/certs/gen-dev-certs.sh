#!/usr/bin/env bash
# Generate locally-trusted TLS certs for local development, using mkcert:
#   - app:     Caddy's cert, covering the panel at https://localhost:8443 and every
#              deployment route at https://<slug>.dinchy.localhost:8443
#   - mailpit: Mailpit's STARTTLS cert (the app connects with mandatory STARTTLS)
#   - ca.pem:  a copy of mkcert's CA root
#
# mkcert is the single local certificate authority: `mkcert -install` adds its root to
# the system and browser trust stores, so Caddy's HTTPS and Mailpit's cert are trusted
# with no warning. Production has a real domain and uses Caddy's ACME automation
# instead — Let's Encrypt cannot validate localhost, which is the whole reason the two
# environments differ here.
#
# Deployment hostnames sit under dinchy.localhost rather than directly under localhost
# because TLS clients reject a wildcard whose parent is a single label: `*.localhost` is
# treated like `*.com` and refused (verified with curl/OpenSSL), while
# `*.dinchy.localhost` is accepted. Both names still resolve to loopback through
# nss-myhostname and the browsers' built-in .localhost handling.
#
# `localhost` is listed separately because a wildcard covers exactly one label and never
# matches the bare parent name.
#
# Postgres and Redis run plaintext on loopback in dev, so they need no certs here.
#
# Run once before `mise run dev` / `mise run infra:up`; re-run only to regenerate.
set -euo pipefail

CERT_DIR="deploy/certs"

mkcert -install
mkdir -p "$CERT_DIR"

gen() {
	local name="$1"
	shift
	mkcert -cert-file "$CERT_DIR/$name.pem" -key-file "$CERT_DIR/$name-key.pem" "$@"
	chmod 600 "$CERT_DIR/$name-key.pem"
}

gen app localhost '*.dinchy.localhost' 127.0.0.1 ::1
gen mailpit mailpit localhost 127.0.0.1

cp "$(mkcert -CAROOT)/rootCA.pem" "$CERT_DIR/ca.pem"

echo "Wrote dev certs to $CERT_DIR/ (trusted via mkcert's local CA)."
