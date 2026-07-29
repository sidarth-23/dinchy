#!/usr/bin/env bash
# Build Caddy with the plugins declared in deploy/caddy/plugins.txt.
#
# Letting an operator compile their own Caddy is a supported feature, not a packaging
# detail, so this does more than shell out to xcaddy: it refuses an unpinned manifest,
# and it verifies that every requested plugin actually registered a Caddy module before
# installing the result. xcaddy happily produces a binary when a --with module resolves
# as a Go module but registers nothing, so "the build succeeded" is not the same as
# "the plugin is loaded".
#
# Requires a Go toolchain, git, and network access. Dinchy deliberately trades the
# single-binary install for this: extensibility is worth more than one fewer artifact.
#
# Usage:
#   mise run caddy:build                     # writes tmp/caddy
#   DINCHY_CADDY_OUTPUT=/usr/local/bin/caddy mise run caddy:build
#   GOOS=linux GOARCH=arm64 mise run caddy:build
set -euo pipefail

MANIFEST="${DINCHY_CADDY_PLUGINS:-deploy/caddy/plugins.txt}"
OUTPUT="${DINCHY_CADDY_OUTPUT:-tmp/caddy}"

if [[ ! -f $MANIFEST ]]; then
	echo "error: plugin manifest not found: $MANIFEST" >&2
	exit 1
fi

caddy_version=""
declare -a with_args=()
declare -a plugin_modules=()

line_number=0
while IFS= read -r raw || [[ -n $raw ]]; do
	line_number=$((line_number + 1))
	entry="${raw%%#*}"
	# Strip surrounding whitespace without touching the module path itself.
	entry="$(printf '%s' "$entry" | tr -d '[:space:]')"
	[[ -z $entry ]] && continue

	if [[ $entry != *@* ]]; then
		echo "error: $MANIFEST:$line_number: '$entry' has no version." >&2
		echo "       Pin it as module@version — an unpinned entry makes the build unreproducible." >&2
		exit 1
	fi

	module="${entry%@*}"
	version="${entry##*@}"
	if [[ -z $module || -z $version ]]; then
		echo "error: $MANIFEST:$line_number: '$entry' is not a valid module@version entry." >&2
		exit 1
	fi

	if [[ $module == "caddy" ]]; then
		caddy_version="$version"
		continue
	fi

	with_args+=("--with" "$entry")
	plugin_modules+=("$module")
done <"$MANIFEST"

if [[ -z $caddy_version ]]; then
	echo "error: $MANIFEST has no 'caddy@<version>' entry pinning the Caddy version." >&2
	exit 1
fi

staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT
candidate="$staging/caddy"

echo "==> Building Caddy $caddy_version with ${#plugin_modules[@]} plugin(s)"
# CGO_ENABLED=0 keeps the binary static, matching the deliberately cgo-free main binary.
CGO_ENABLED=0 xcaddy build "$caddy_version" --output "$candidate" "${with_args[@]}"

echo "==> Verifying the build"
"$candidate" version

modules_json="$("$candidate" list-modules --json)"
missing=()
for module in "${plugin_modules[@]}"; do
	# Match the Go module path in the package field rather than a module ID, since one
	# plugin can register several differently named Caddy modules.
	if ! printf '%s' "$modules_json" | grep -q -- "$module"; then
		missing+=("$module")
	fi
done

if ((${#missing[@]} > 0)); then
	echo "error: the build succeeded but these plugins registered no Caddy module:" >&2
	for module in "${missing[@]}"; do
		echo "       - $module" >&2
	done
	echo "       Check the module path in $MANIFEST; the binary was not installed." >&2
	exit 1
fi

mkdir -p "$(dirname "$OUTPUT")"

# Replacing the file drops any file capability set on it, so a production binary that
# was granted CAP_NET_BIND_SERVICE loses the ability to bind :80/:443 and Caddy fails to
# start after the swap. Warn while the reason is still obvious.
had_bind_capability=false
if command -v getcap >/dev/null 2>&1 && [[ -e $OUTPUT ]]; then
	if getcap "$OUTPUT" 2>/dev/null | grep -q cap_net_bind_service; then
		had_bind_capability=true
	fi
fi

printf '%s' "$modules_json" >"$staging/modules.json"
mv "$candidate" "$OUTPUT"
mv "$staging/modules.json" "$OUTPUT.modules.json"

echo "==> Installed $OUTPUT"
for module in "${plugin_modules[@]}"; do
	echo "    plugin: $module"
done

if [[ $had_bind_capability == true ]]; then
	echo
	echo "warning: the previous binary held CAP_NET_BIND_SERVICE and the new one does not." >&2
	echo "         Caddy will fail to bind :80/:443 until you restore it:" >&2
	echo "           sudo setcap cap_net_bind_service=+ep $OUTPUT" >&2
	echo "         Or prefer the sysctl, which survives every rebuild:" >&2
	echo "           sudo sysctl -w net.ipv4.ip_unprivileged_port_start=80" >&2
fi
