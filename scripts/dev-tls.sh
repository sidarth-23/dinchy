#!/usr/bin/env bash
# The parts of local TLS setup that are genuinely shell. Taskfile.yml owns the ordering and the
# "is this already done" checks; this file holds the per-platform knowledge — where certutil
# comes from on each distribution, how the browser's NSS database works, and how to read a CA
# root out of a Caddy that has to be running to answer.
#
# Every subcommand does one thing and fails if it cannot. Setup is interactive by design
# (`task setup`, from a terminal), so a step that needs a password asks for one, and a step that
# cannot finish is an error rather than a warning — a trust store reporting success it does not
# have is the failure mode this file exists to prevent.
#
# Usage:
#   dev-tls.sh certutil                        install certutil for this distribution
#   dev-tls.sh nssdb                           create an empty browser certificate database
#   dev-tls.sh nss-add <root> <nickname>       add a CA root to the browser database
#   dev-tls.sh trust-caddy                     put Caddy's root in the system trust store
#   dev-tls.sh is-trusted system <root>        exit 0 if the root is in the system store
#   dev-tls.sh is-trusted browser <nickname>   exit 0 if the root is in the browser store
set -euo pipefail

NSS_DB="$HOME/.pki/nssdb"
CADDY_BIN="${CADDY_BIN:-tmp/caddy}"
# The admin API as reached from this machine. Deliberately not DINCHY_CADDY_ADMIN_ENDPOINT: that is
# the app's route to the same endpoint over the container network (caddy-edge:2019), which does not
# resolve here. Taskfile.yml passes the published loopback address.
CADDY_ADMIN="${DINCHY_DEV_CADDY_ADMIN:-127.0.0.1:2019}"

if [[ $EUID -eq 0 ]]; then
	SUDO=()
else
	SUDO=(sudo)
fi

# nss_tools_package names the package manager and the package holding certutil for this machine,
# printing nothing when the manager is unknown.
nss_tools_package() {
	if command -v apt-get >/dev/null 2>&1; then
		echo "apt-get libnss3-tools"
	elif command -v dnf >/dev/null 2>&1; then
		echo "dnf nss-tools"
	elif command -v pacman >/dev/null 2>&1; then
		echo "pacman nss"
	elif command -v zypper >/dev/null 2>&1; then
		echo "zypper mozilla-nss-tools"
	elif command -v brew >/dev/null 2>&1; then
		echo "brew nss"
	fi
}

# install_certutil installs certutil through this machine's package manager.
#
# Chrome and Firefox read certificate trust from their own NSS database rather than the system
# store, and both `mkcert -install` and `caddy trust` populate it only through certutil. Without
# it the roots install everywhere except the browsers, which is the one place the certificates
# are actually looked at.
install_certutil() {
	local manager="" package=""
	read -r manager package <<<"$(nss_tools_package)" || true
	if [[ -z "$manager" ]]; then
		echo "certutil is missing and no known package manager is available. Install your" >&2
		echo "distribution's NSS tools and re-run 'task tls', or run 'task tls:system' to set" >&2
		echo "up the system trust store alone and accept a certificate warning in the browser." >&2
		return 1
	fi

	echo "Installing $package so Chrome and Firefox pick up the development CA roots..."
	# Homebrew installs without root; every other manager here needs it.
	case "$manager" in
	apt-get) "${SUDO[@]}" apt-get install -y "$package" ||
		{ "${SUDO[@]}" apt-get update -qq && "${SUDO[@]}" apt-get install -y "$package"; } ;;
	dnf) "${SUDO[@]}" dnf install -y "$package" ;;
	pacman) "${SUDO[@]}" pacman -S --needed --noconfirm "$package" ;;
	zypper) "${SUDO[@]}" zypper --non-interactive install "$package" ;;
	brew) brew install "$package" ;;
	esac
}

# create_nss_db creates an empty browser certificate database.
#
# Both mkcert and caddy install into an NSS database only if one already exists, and one exists
# only once a browser has created it. On a machine where Chrome and Firefox have not run yet,
# every root therefore lands in the system store and silently nowhere else — which is the case
# that looks like "trust is installed" right up until the browser rejects the page. An empty,
# password-less database is exactly what a browser would create, so making it here is safe.
create_nss_db() {
	mkdir -p "$NSS_DB"
	if ! certutil -d "sql:$NSS_DB" -N --empty-password >/dev/null 2>&1; then
		echo "Could not create the browser certificate database at $NSS_DB." >&2
		return 1
	fi
	echo "Created an empty browser certificate database at $NSS_DB."
}

# add_root_to_nss adds a CA root to the browser database. The database belongs to the user, so
# this needs no privileges.
add_root_to_nss() {
	local root="$1" nickname="$2"
	if [[ ! -f "$root" ]]; then
		echo "No CA root at $root. Caddy generates it on first run — 'task tls' does that" >&2
		echo "for you as part of installing the root into the system store." >&2
		return 1
	fi
	certutil -d "sql:$NSS_DB" -A -t "C,," -n "$nickname" -i "$root"
	echo "Added \"$nickname\" to the browser trust store (restart the browser)."
}

# caddy_admin_up reports whether Caddy's admin API is accepting connections. A bare TCP connect
# is enough and keeps this free of a curl dependency.
caddy_admin_up() {
	local host="${CADDY_ADMIN%:*}" port="${CADDY_ADMIN##*:}"
	(exec 3<>"/dev/tcp/$host/$port") 2>/dev/null
}

# trust_caddy installs Caddy's root into the system store, starting Caddy first when nothing is
# listening.
#
# `caddy trust` reads the certificate from the admin API rather than from disk, so it fails
# outright when Caddy is not running — and setup has to work before the edge is up, because that
# is what setup is. Caddy is started with no configuration at all: the admin endpoint alone is
# enough to serve the CA, it generates the authority on demand, and it binds no other port, so
# this cannot collide with whatever is already using :8443. Anything already listening is left
# alone and simply used.
trust_caddy() {
	local started="" attempt
	if ! caddy_admin_up; then
		"$CADDY_BIN" run >/dev/null 2>&1 &
		started=$!
		for attempt in $(seq 1 20); do
			caddy_admin_up && break
			sleep 1
		done
		if ! caddy_admin_up; then
			echo "Caddy's admin API never came up on $CADDY_ADMIN, so its CA cannot be read." >&2
			kill "$started" 2>/dev/null || true
			return 1
		fi
	fi

	local status=0
	"$CADDY_BIN" trust || status=1

	if [[ -n "$started" ]]; then
		kill "$started" 2>/dev/null || true
		wait "$started" 2>/dev/null || true
	fi
	return "$status"
}

# is_trusted_system reports whether a CA root is installed in the system store, by verifying the
# self-signed root against the store: it only chains if the store already contains it.
#
# The bundle is named explicitly rather than left to openssl's default. A bare `openssl verify`
# resolves its trust directory from the OPENSSLDIR of whichever openssl is first on PATH, so a
# Homebrew openssl makes every root look untrusted no matter what is installed — which reads as
# "setup never finished" and re-runs the privileged steps on every invocation.
is_trusted_system() {
	local root="$1" bundle
	[[ -f "$root" ]] || return 1

	if [[ "$(uname -s)" == "Darwin" ]]; then
		local sha1
		sha1=$(openssl x509 -in "$root" -noout -fingerprint -sha1 | cut -d= -f2 | tr -d :) || return 1
		security find-certificate -a -Z /Library/Keychains/System.keychain 2>/dev/null |
			grep -qi "SHA-1 hash: $sha1"
		return
	fi

	for bundle in /etc/pki/tls/certs/ca-bundle.crt /etc/ssl/certs/ca-certificates.crt /etc/ssl/cert.pem; do
		[[ -f "$bundle" ]] || continue
		openssl verify -CAfile "$bundle" "$root" >/dev/null 2>&1
		return
	done

	echo "No system CA bundle found in any known location; cannot tell whether $root is" >&2
	echo "trusted. Install the roots by hand, or run 'task tls:system'." >&2
	return 1
}

# is_trusted_browser reports whether a CA root is installed in the browser database. A missing
# certutil or database means there is nothing to check and nothing to do, so both count as
# satisfied — the browser half is optional, and `task tls:certutil` is what reports it missing.
is_trusted_browser() {
	command -v certutil >/dev/null 2>&1 || return 0
	[[ -d "$NSS_DB" ]] || return 0
	certutil -d "sql:$NSS_DB" -L 2>/dev/null | grep -q "$1"
}

case "${1:-}" in
certutil) install_certutil ;;
nssdb) create_nss_db ;;
nss-add) add_root_to_nss "${2:?root path required}" "${3:?nickname required}" ;;
trust-caddy) trust_caddy ;;
is-trusted)
	case "${2:-}" in
	system) is_trusted_system "${3:?root path required}" ;;
	browser) is_trusted_browser "${3:?nickname required}" ;;
	*)
		echo "usage: dev-tls.sh is-trusted <system|browser> <root|nickname>" >&2
		exit 2
		;;
	esac
	;;
*)
	echo "usage: dev-tls.sh <certutil|nssdb|nss-add|trust-caddy|is-trusted>" >&2
	exit 2
	;;
esac
