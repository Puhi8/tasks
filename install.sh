#!/bin/sh
# Installs tasks, a terminal editor for CalDAV (Tasks.org) tasks.
#
#   install.sh               install the latest release (Linux, x86-64 or ARM)
#   install.sh --source      build and install from this checkout (needs Go)
#   install.sh --uninstall   remove the program; config and tasks are kept
#
# TASKS_TUI_INSTALL_DIR sets where the program goes (default: ~/.local/bin).
set -eu

install_dir="${TASKS_TUI_INSTALL_DIR:-$HOME/.local/bin}"
config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/tasks-tui"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

case "${1:-}" in
--uninstall)
	rm -f "$install_dir/tasks" "$config_dir/config.example.toml"
	echo "removed $install_dir/tasks; your config and tasks are kept"
	exit 0
	;;
--source)
	cd "$(dirname "$0")"
	version="$(git describe --tags --always 2>/dev/null || echo dev)"
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$workdir/tasks" ./cmd/tasks
	cp config.example.toml "$workdir/"
	;;
"")
	[ "$(uname -s)" = Linux ] || fail "release builds are for Linux only, use --source"
	case "$(uname -m)" in
	x86_64) architecture=amd64 ;;
	aarch64) architecture=arm64 ;;
	*) fail "no release build for $(uname -m), use --source" ;;
	esac
	url="https://github.com/puhi8/tasks/releases/latest/download/tasks-linux-$architecture.tar.gz"
	curl -fsSL "$url" | tar -xz -C "$workdir" || fail "could not download $url"
	;;
*)
	fail "unknown option $1 (use --source or --uninstall)"
	;;
esac

mkdir -p "$install_dir" "$config_dir"
install -m 755 "$workdir/tasks" "$install_dir/tasks"
install -m 644 "$workdir/config.example.toml" "$config_dir/config.example.toml"
echo "installed tasks $("$install_dir/tasks" --version) to $install_dir"
[ -f "$config_dir/config.toml" ] || echo "to sync with a server, copy $config_dir/config.example.toml to config.toml and fill in [server]"
case ":$PATH:" in
*":$install_dir:"*) ;;
*) echo "note: $install_dir is not in your PATH" ;;
esac
