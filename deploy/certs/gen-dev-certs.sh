#!/usr/bin/env bash
# Generate locally-trusted TLS certs for local development, using mkcert:
#   - app:     the Go server's own HTTPS cert (browser hits https://localhost:8443)
#   - mailpit: Mailpit's STARTTLS cert (the app connects with mandatory STARTTLS)
#   - ca.pem:  a copy of mkcert's CA root
# `mkcert -install` adds the CA to the system (and browser) trust store, so the Go
# server's HTTPS is trusted with no warning and the app trusts Mailpit's cert.
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

gen app localhost 127.0.0.1
gen mailpit mailpit localhost 127.0.0.1

cp "$(mkcert -CAROOT)/rootCA.pem" "$CERT_DIR/ca.pem"

echo "Wrote dev certs to $CERT_DIR/ (trusted via mkcert's local CA)."
