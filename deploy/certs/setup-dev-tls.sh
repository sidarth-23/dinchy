#!/usr/bin/env bash
# Local development TLS setup: put both development CA roots into every trust store this
# machine has, and issue the one certificate Mailpit cannot issue for itself.
#
# `mise install` runs this through its postinstall hook, so a fresh checkout is ready with no
# separate step. Every action tests for its own result first, so a repeat run does nothing and
# asks for nothing — that idempotence is what makes it safe to hang off `mise install`.
#
# Development has two local authorities because they solve different problems: Caddy signs the
# app's certificates with its own internal CA, and mkcert issues Mailpit's, which Mailpit
# cannot generate. Production has neither, because it uses ACME with a real domain.
#
# Installing a root needs root privileges, so each step that needs them is skipped with an
# explanation, and the script still exits 0, rather than failing the install it hangs off.
# Set DINCHY_SKIP_TLS_SETUP=1 to skip it unconditionally.
set -euo pipefail

CERT_DIR="deploy/certs"
CADDY_BIN="tmp/caddy"
CADDY_ADMIN="${DINCHY_CADDY_ADMIN_ENDPOINT:-127.0.0.1:2019}"
DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
CADDY_ROOT="$DATA_HOME/caddy/pki/authorities/local/root.crt"
MKCERT_ROOT="${CAROOT:-$DATA_HOME/mkcert}/rootCA.pem"
NSS_DB="$HOME/.pki/nssdb"

if [[ -n "${DINCHY_SKIP_TLS_SETUP:-}" ]]; then
	echo "Local TLS setup skipped (DINCHY_SKIP_TLS_SETUP is set)."
	exit 0
fi

# privileged_ok reports whether this run can complete an action that needs root.
#
# sudo reads its password from /dev/tty and not from stdin, so whether stdin is a terminal says
# nothing about whether it can ask. What matters is whether root is already available — running
# as root, or sudo needing no password — and failing that, whether a terminal exists to type one
# into. CI is excluded deliberately: an ephemeral machine has no use for a trust store, and
# prompting on one is a hang waiting to happen.
privileged_ok() {
	[[ -z "${CI:-}" ]] || return 1
	[[ $EUID -eq 0 ]] && return 0
	command -v sudo >/dev/null 2>&1 || return 1
	sudo -n true 2>/dev/null && return 0
	(exec 3<>/dev/tty) 2>/dev/null
}

if [[ $EUID -eq 0 ]]; then
	SUDO=()
else
	SUDO=(sudo)
fi

# system_trusted reports whether a CA root is installed in the system store, which is the half
# that needs root to write. openssl reads that store when given no arguments, so a self-signed
# root that verifies against it is one that has been installed there.
system_trusted() {
	[[ -f "$1" ]] || return 1
	openssl verify "$1" >/dev/null 2>&1
}

# nss_trusted reports whether a CA root is installed in the browser store, which is a per-user
# database and needs no privileges to write. A missing certutil or database means there is
# nothing to check and nothing to do, so both count as satisfied.
nss_trusted() {
	command -v certutil >/dev/null 2>&1 || return 0
	[[ -d "$NSS_DB" ]] || return 0
	certutil -d "sql:$NSS_DB" -L 2>/dev/null | grep -q "$1"
}

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

# ensure_certutil installs certutil when it is missing, reporting non-zero when it could not.
#
# Chrome and Firefox read certificate trust from their own NSS database rather than the system
# store, and both `mkcert -install` and `caddy trust` populate it only through certutil. Without
# it the roots install everywhere except the browsers, which is the one place the certificates
# are actually looked at. Every failure here is reported and tolerated: the system store still
# gets both roots, so curl and the Go SMTP client work either way.
ensure_certutil() {
	if command -v certutil >/dev/null 2>&1; then
		return 0
	fi

	local manager="" package=""
	read -r manager package <<<"$(nss_tools_package)" || true
	if [[ -z "$manager" ]]; then
		echo "certutil not found and no known package manager; install your distribution's NSS" >&2
		echo "tools so Chrome and Firefox trust the development roots, then: mise run dev:tls" >&2
		return 1
	fi

	# Homebrew installs without root; every other manager here needs it.
	if [[ "$manager" != "brew" ]] && ! privileged_ok; then
		echo "certutil is missing, so Chrome and Firefox will not trust the development roots." >&2
		echo "Installing $package needs root and nothing here can authenticate; run" >&2
		echo "'mise run dev:tls' from a terminal to finish this." >&2
		return 1
	fi

	echo "Installing $package so Chrome and Firefox pick up the development CA roots..."
	local failed=0
	case "$manager" in
	apt-get) "${SUDO[@]}" apt-get install -y "$package" || { "${SUDO[@]}" apt-get update -qq && "${SUDO[@]}" apt-get install -y "$package"; } || failed=1 ;;
	dnf) "${SUDO[@]}" dnf install -y "$package" || failed=1 ;;
	pacman) "${SUDO[@]}" pacman -S --needed --noconfirm "$package" || failed=1 ;;
	zypper) "${SUDO[@]}" zypper --non-interactive install "$package" || failed=1 ;;
	brew) brew install "$package" || failed=1 ;;
	esac

	if ((failed != 0)); then
		echo "Could not install $package; the system store is still set up, browsers are not." >&2
		return 1
	fi
}

# ensure_nss_db creates an empty browser certificate database when there is none.
#
# Both mkcert and caddy install into an NSS database only if one already exists, and one exists
# only once a browser has created it. On a machine where Chrome and Firefox have not run yet,
# every root therefore lands in the system store and silently nowhere else — which is the case
# that looks like "trust is installed" right up until the browser rejects the page. An empty,
# password-less database is exactly what a browser would create, so making it here is safe.
ensure_nss_db() {
	command -v certutil >/dev/null 2>&1 || return 0
	[[ -d "$NSS_DB" ]] && return 0

	mkdir -p "$NSS_DB"
	if ! certutil -d "sql:$NSS_DB" -N --empty-password >/dev/null 2>&1; then
		echo "Could not create the browser certificate database at $NSS_DB;" >&2
		echo "the system store is still set up, browsers are not." >&2
		return 1
	fi
	echo "Created an empty browser certificate database at $NSS_DB."
}

# install_root_into_nss adds a CA root to the browser database directly.
#
# `caddy trust` stops as soon as it finds its root in the system store and never looks at the
# browser database, so a machine that already has the system half keeps failing in Chrome with
# nothing reporting why. The database belongs to the user, so writing it needs no privileges.
install_root_into_nss() {
	local root="$1" nickname="$2"
	command -v certutil >/dev/null 2>&1 || return 0
	[[ -d "$NSS_DB" ]] || return 0
	[[ -f "$root" ]] || return 1
	certutil -d "sql:$NSS_DB" -A -t "C,," -n "$nickname" -i "$root"
}

# caddy_admin_up reports whether Caddy's admin API is accepting connections. A bare TCP connect
# is enough and keeps this free of a curl dependency.
caddy_admin_up() {
	local host="${CADDY_ADMIN%:*}" port="${CADDY_ADMIN##*:}"
	(exec 3<>"/dev/tcp/$host/$port") 2>/dev/null
}

# trust_caddy_root installs Caddy's root, starting Caddy first when nothing is listening.
#
# `caddy trust` reads the certificate from the admin API rather than from disk, so it fails
# outright when Caddy is not running — and setup has to work before the edge is up, because that
# is what setup is. Caddy is started with no configuration at all: the admin endpoint alone is
# enough to serve the CA, it generates the authority on demand, and it binds no other port, so
# this cannot collide with whatever is already using :8443. Anything already listening is left
# alone and simply used.
trust_caddy_root() {
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

deferred=""

if ! ensure_certutil; then
	deferred="yes"
fi

if ! ensure_nss_db; then
	deferred="yes"
fi

# mkcert's own root, plus Mailpit's certificate. The generator is idempotent, so this settles
# whatever is outstanding without a second condition here. Only the system store half needs
# root, so once that is in place the remaining browser-store work proceeds unprivileged.
if system_trusted "$MKCERT_ROOT" || privileged_ok; then
	bash "$CERT_DIR/gen-dev-certs.sh"
else
	echo "mkcert's root is not in the system store and nothing here can authenticate as root;" >&2
	echo "Mailpit's certificate is pending. Run 'mise run dev:tls' from a terminal." >&2
	deferred="yes"
fi

# Caddy's root, in two independent halves: the system store, which needs root and where
# `caddy trust` knows the per-platform details, and the browser store, which does not.
if system_trusted "$CADDY_ROOT"; then
	echo "Caddy's local CA root is in the system trust store."
elif privileged_ok; then
	echo "Installing Caddy's local CA root into the system trust store..."
	trust_caddy_root || deferred="yes"
else
	echo "Caddy's local CA root is not in the system store and nothing here can authenticate" >&2
	echo "as root; run 'mise run dev:tls' from a terminal." >&2
	deferred="yes"
fi

if nss_trusted "Caddy"; then
	echo "Caddy's local CA root is in the browser trust store."
elif install_root_into_nss "$CADDY_ROOT" "Caddy Local Authority"; then
	echo "Added Caddy's local CA root to the browser trust store (restart the browser)."
else
	echo "Could not add Caddy's local CA root to the browser trust store at $NSS_DB." >&2
	deferred="yes"
fi

# Report what is true rather than what was attempted: a step that ran is not a step that worked,
# and a trust store reporting success it does not have is the failure mode this whole script
# exists to prevent.
if [[ -n "$deferred" ]] || ! system_trusted "$CADDY_ROOT" || ! nss_trusted "Caddy"; then
	echo "Local TLS setup is incomplete — see the notes above. Re-run 'mise run dev:tls'."
	exit 0
fi

echo "Local TLS setup complete: https://localhost:8443 is trusted, Mailpit's cert is in $CERT_DIR/."
