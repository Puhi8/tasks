#!/bin/sh
# Installs tasks, a terminal editor for CalDAV (Tasks.org) tasks.
#
#   install.sh               install the latest release
#   install.sh --source      build and install from this checkout (needs Go)
#   install.sh --uninstall   remove the program; config and tasks are kept
#
# TASKS_TUI_VERSION      release to install, e.g. v1.2.3 (default: latest)
# TASKS_TUI_INSTALL_DIR  where the program goes (default: ~/.local/bin)
# TASKS_TUI_REPOSITORY   owner/name on GitHub (filled in for the copy attached to a release)
set -eu

repository="${TASKS_TUI_REPOSITORY:-__REPOSITORY__}"
version="${TASKS_TUI_VERSION:-latest}"
install_dir="${TASKS_TUI_INSTALL_DIR:-$HOME/.local/bin}"
config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/tasks-tui"
data_dir="${XDG_DATA_HOME:-$HOME/.local/share}/tasks-tui"

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$2" "$1"
	else
		fail "curl or wget is needed"
	fi
}

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

case "${1:-}" in
--uninstall)
	rm -f "$install_dir/tasks" "$config_dir/config.example.toml"
	echo "removed $install_dir/tasks; your config ($config_dir) and tasks ($data_dir) are kept"
	exit 0
	;;
--source)
	source_dir="$(cd "$(dirname "$0")" && pwd)"
	command -v go >/dev/null 2>&1 || fail "--source needs Go (https://go.dev/dl)"
	source_version="$(git -C "$source_dir" describe --tags --always 2>/dev/null || echo dev)"
	echo "building $source_version from $source_dir"
	(cd "$source_dir" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$source_version" -o "$workdir/tasks" ./cmd/tasks)
	cp "$source_dir/config.example.toml" "$workdir/"
	;;
"")
	case "$repository" in
	*/*) ;;
	*) fail "unknown GitHub repository: use the install.sh attached to a release, set TASKS_TUI_REPOSITORY=owner/name or run with --source" ;;
	esac
	case "$(uname -s)" in
	Linux) system=linux ;;
	Darwin) system=darwin ;;
	*) fail "no release build for $(uname -s), try --source" ;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) architecture=amd64 ;;
	aarch64 | arm64) architecture=arm64 ;;
	*) fail "no release build for $(uname -m), try --source" ;;
	esac
	archive="tasks-$system-$architecture.tar.gz"
	if [ "$version" = latest ]; then
		base="https://github.com/$repository/releases/latest/download"
	else
		base="https://github.com/$repository/releases/download/$version"
	fi
	echo "downloading $archive ($version) from $repository"
	download "$base/$archive" "$workdir/$archive" || fail "could not download $base/$archive"
	download "$base/checksums.txt" "$workdir/checksums.txt" || fail "could not download the checksums"
	# a corrupted or tampered download must never be installed
	(
		cd "$workdir"
		grep " $archive\$" checksums.txt >expected.txt || exit 1
		sha256sum -c expected.txt >/dev/null 2>&1 || shasum -a 256 -c expected.txt >/dev/null 2>&1
	) || fail "checksum of $archive does not match, nothing was installed"
	tar -xzf "$workdir/$archive" -C "$workdir"
	;;
*)
	fail "unknown option $1 (use --source or --uninstall)"
	;;
esac

mkdir -p "$install_dir" "$config_dir"
install -m 755 "$workdir/tasks" "$install_dir/tasks"
install -m 644 "$workdir/config.example.toml" "$config_dir/config.example.toml"
echo "installed tasks $("$install_dir/tasks" --version) to $install_dir/tasks"

if [ ! -f "$config_dir/config.toml" ]; then
	echo "to sync with a server: cp $config_dir/config.example.toml $config_dir/config.toml and fill in [server]"
	echo "without a config the app works local-only"
fi
case ":$PATH:" in
*":$install_dir:"*) ;;
*) echo "note: $install_dir is not in your PATH; add it in your shell profile to run tasks from anywhere" ;;
esac
