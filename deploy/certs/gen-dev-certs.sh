#!/usr/bin/env bash
# Issue Mailpit's STARTTLS certificate for local development, using mkcert.
#
# Mailpit only, and only because it cannot generate one itself: it serves STARTTLS from a
# cert file or not at all, and the app connects with mandatory STARTTLS in every
# environment. `mkcert -install` puts its root in the system trust store, which is how the
# Go SMTP client verifies the certificate — it has no CA-file option.
#
# Caddy needs nothing from here. It signs its own development certificates with its
# internal CA, per host, on demand; `mise run caddy:trust` installs that root. Production
# has a real domain and uses ACME, which cannot validate localhost.
#
# Postgres and Redis run plaintext on loopback in dev, so they need no certs.
#
# Run once before `mise run infra:up`; re-run only to regenerate.
set -euo pipefail

CERT_DIR="deploy/certs"

mkcert -install
mkdir -p "$CERT_DIR"

mkcert -cert-file "$CERT_DIR/mailpit.pem" -key-file "$CERT_DIR/mailpit-key.pem" \
	mailpit localhost 127.0.0.1
chmod 600 "$CERT_DIR/mailpit-key.pem"

echo "Wrote Mailpit's dev cert to $CERT_DIR/ (trusted via mkcert's local CA)."
