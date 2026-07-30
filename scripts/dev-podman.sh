#!/usr/bin/env bash
# The parts of container-runtime setup that are genuinely shell. Taskfile.yml owns the ordering
# and the "is this already done" checks; this file holds the per-platform knowledge — where
# podman comes from on each distribution, which rootless prerequisites it cannot supply itself,
# and how to tell a working installation from a present one.
#
# podman is not a pinned tool. mise installs binaries into its own store and has no side
# effects, but rootless podman needs subordinate UID ranges in /etc/subuid and a user systemd
# session that outlives the login — machine state a version-pinned download cannot provide. That
# split is why this is a setup step and why it is allowed to ask for a password.
#
# Every subcommand does one thing and fails if it cannot. A runtime that is installed but cannot
# start a rootless container is the failure mode this file exists to catch: it looks like success
# until the first `podman-compose up` and then reports something unrelated.
#
# Usage:
#   dev-podman.sh install     install podman and the rootless prerequisites
#   dev-podman.sh verify      report every requirement, with what to do about each
#   dev-podman.sh is-ready    exit 0 if podman is installed and usable, printing nothing
set -euo pipefail

# Below this, podman ignores `depends_on: condition: service_healthy` with only a warning, so a
# migration races a cold database and the failure surfaces as a missing table.
MIN_PODMAN_VERSION="4.6"

# The range systemd-homed and most distributions hand out per user. 65536 is the smallest range
# that covers a container image's full UID space.
SUBID_START="100000"
SUBID_COUNT="65536"

if [[ $EUID -eq 0 ]]; then
	SUDO=()
else
	SUDO=(sudo)
fi

# podman_package names the package manager and the packages holding podman and its rootless
# networking stack for this machine, printing nothing when the manager is unknown.
#
# The extra packages are not optional where they are named. Debian splits the setuid helpers that
# map subordinate IDs into uidmap, and Arch ships netavark and aardvark-dns separately — without
# aardvark-dns a container network resolves no names, which is exactly what this project's
# service-to-service addressing depends on.
podman_package() {
	if command -v apt-get >/dev/null 2>&1; then
		echo "apt-get podman uidmap slirp4netns"
	elif command -v dnf >/dev/null 2>&1; then
		echo "dnf podman"
	elif command -v pacman >/dev/null 2>&1; then
		echo "pacman podman netavark aardvark-dns"
	elif command -v zypper >/dev/null 2>&1; then
		echo "zypper podman"
	elif command -v brew >/dev/null 2>&1; then
		echo "brew podman"
	fi
}

# install_podman installs podman through this machine's package manager, then supplies the two
# things the package cannot: subordinate ID ranges and a lingering user session.
install_podman() {
	if [[ "$(uname -s)" == "Darwin" ]]; then
		echo "On macOS podman runs containers inside a virtual machine, which this project's" >&2
		echo "development setup does not support: the stack bind-mounts the repository at its" >&2
		echo "host path so the debugger's source paths resolve, and shares the host's Caddy" >&2
		echo "data directory so the browser trusts the local CA. Neither survives the VM" >&2
		echo "boundary. Use a Linux machine or a Linux VM you develop inside." >&2
		return 1
	fi

	if ! command -v podman >/dev/null 2>&1; then
		local manager="" packages=""
		read -r manager packages <<<"$(podman_package)" || true
		if [[ -z "$manager" ]]; then
			echo "podman is missing and no known package manager is available. Install podman and" >&2
			echo "its rootless networking stack for your distribution, then re-run 'task podman'." >&2
			return 1
		fi

		echo "Installing $packages so the development stack can run in containers..."
		# Homebrew installs without root; every other manager here needs it. apt-get is retried
		# after an update because a package index older than the archive fails to resolve.
		#
		# shellcheck disable=SC2086 # $packages is a word-split list, not one package name.
		case "$manager" in
		apt-get) "${SUDO[@]}" apt-get install -y $packages ||
			{ "${SUDO[@]}" apt-get update -qq && "${SUDO[@]}" apt-get install -y $packages; } ;;
		dnf) "${SUDO[@]}" dnf install -y $packages ;;
		pacman) "${SUDO[@]}" pacman -S --needed --noconfirm $packages ;;
		zypper) "${SUDO[@]}" zypper --non-interactive install $packages ;;
		esac
	fi

	install_subid_ranges
	enable_linger
}

# install_subid_ranges gives the user subordinate UID and GID ranges, which is what lets a
# rootless container map a UID other than the user's own.
#
# Without these every rootless container fails to start, reporting a missing newuidmap range
# rather than anything about the container. `podman system migrate` afterwards is required, not
# tidiness: podman caches the mapping in its per-user storage and keeps using the old one.
install_subid_ranges() {
	if subid_ranges_present; then
		return 0
	fi

	echo "Adding subordinate UID and GID ranges for $USER, which rootless podman requires..."
	"${SUDO[@]}" usermod \
		--add-subuids "$SUBID_START-$((SUBID_START + SUBID_COUNT - 1))" \
		--add-subgids "$SUBID_START-$((SUBID_START + SUBID_COUNT - 1))" \
		"$USER"
	podman system migrate
}

# enable_linger keeps the user's systemd session alive after logout, so a container that is meant
# to outlive a terminal actually does.
#
# The shared Caddy edge is long-lived and shared with every other project on this machine. Without
# linger, logging out stops it and takes every project's routing with it.
enable_linger() {
	if linger_enabled; then
		return 0
	fi
	echo "Enabling systemd linger for $USER, so long-lived containers survive logout..."
	"${SUDO[@]}" loginctl enable-linger "$USER"
}

# subid_ranges_present reports whether the user has subordinate ID ranges in both databases.
#
# Both files are read, not just subuid: a user with UID ranges and no GID ranges fails in the same
# way as one with neither, and it is the harder failure to recognise.
subid_ranges_present() {
	local file
	for file in /etc/subuid /etc/subgid; do
		[[ -f "$file" ]] || return 1
		grep -q "^$USER:" "$file" || return 1
	done
}

# linger_enabled reports whether the user's systemd session outlives logout.
linger_enabled() {
	command -v loginctl >/dev/null 2>&1 || return 1
	[[ "$(loginctl show-user "$USER" --property=Linger --value 2>/dev/null)" == "yes" ]]
}

# podman_usable reports whether podman can actually inspect its own storage, which is the cheapest
# call that exercises the rootless mapping rather than just the binary's presence.
podman_usable() {
	command -v podman >/dev/null 2>&1 || return 1
	podman info >/dev/null 2>&1
}

# podman_version_ok reports whether podman is new enough for the compose features this project
# relies on. See MIN_PODMAN_VERSION.
podman_version_ok() {
	local version
	version=$(podman version --format '{{.Client.Version}}' 2>/dev/null) || return 1
	[[ -n "$version" ]] || return 1
	# sort -C exits 0 when its input is already ordered, so this is "minimum <= installed".
	printf '%s\n%s\n' "$MIN_PODMAN_VERSION" "$version" | sort -V -C
}

# verify_podman reports every requirement and what to do about each one that is unmet.
#
# Each check prints its own remediation rather than a single generic failure, because the four
# failures need four different actions and three of them need root.
verify_podman() {
	local failed=0

	if podman_usable; then
		echo "podman: $(podman version --format '{{.Client.Version}}')"
	else
		echo "podman is not installed, or cannot read its own storage. Run 'task podman'." >&2
		# Everything below reports on a runtime that is not there, which is noise.
		return 1
	fi

	if ! podman_version_ok; then
		echo "podman $MIN_PODMAN_VERSION or newer is required: below it, container health" >&2
		echo "conditions in compose files are ignored with only a warning, so a migration can" >&2
		echo "race a cold database. Upgrade podman through your package manager." >&2
		failed=1
	fi

	if ! subid_ranges_present; then
		echo "No subordinate ID ranges for $USER in /etc/subuid and /etc/subgid; rootless" >&2
		echo "containers cannot start. Fix with:" >&2
		echo "  sudo usermod --add-subuids $SUBID_START-$((SUBID_START + SUBID_COUNT - 1)) \\" >&2
		echo "    --add-subgids $SUBID_START-$((SUBID_START + SUBID_COUNT - 1)) $USER && podman system migrate" >&2
		failed=1
	fi

	if ! linger_enabled; then
		echo "systemd linger is off for $USER, so the shared Caddy edge stops when you log out" >&2
		echo "and takes every project's routing with it. Fix with:" >&2
		echo "  sudo loginctl enable-linger $USER" >&2
		failed=1
	fi

	if ! command -v podman-compose >/dev/null 2>&1; then
		echo "podman-compose is missing; it is pinned in mise.toml, so 'mise install' installs it." >&2
		failed=1
	fi

	return "$failed"
}

# is_ready is the silent, unprivileged form of verify_podman, for a task's status check.
is_ready() {
	podman_usable &&
		podman_version_ok &&
		subid_ranges_present &&
		linger_enabled &&
		command -v podman-compose >/dev/null 2>&1
}

case "${1:-}" in
install) install_podman ;;
verify) verify_podman ;;
is-ready) is_ready ;;
*)
	echo "usage: dev-podman.sh <install|verify|is-ready>" >&2
	exit 2
	;;
esac
