#!/usr/bin/env bash
# Generate locally-trusted TLS certs for local development, using mkcert. Produces a
# cert/key for the Go server and for each infra container (Postgres/Redis/Mailpit), all
# signed by mkcert's local CA, plus a copy of that CA root for sslrootcert. `mkcert
# -install` adds the CA to the system (and browser) trust store, so the Go server's
# HTTPS is trusted with no warning and the app verifies the infra certs against it.
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
	# Postgres refuses a key readable by group/other; keep all keys owner-only.
	chmod 600 "$CERT_DIR/$name-key.pem"
}

# App cert (browser hits https://localhost:8443); one leaf per infra service, each
# valid for both the compose service name and the loopback host the app dials.
gen app localhost 127.0.0.1
gen postgres postgres localhost 127.0.0.1
gen redis redis localhost 127.0.0.1
gen mailpit mailpit localhost 127.0.0.1

cp "$(mkcert -CAROOT)/rootCA.pem" "$CERT_DIR/ca.pem"

echo "Wrote dev certs to $CERT_DIR/ (trusted via mkcert's local CA)."
