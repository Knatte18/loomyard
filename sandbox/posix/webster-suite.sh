#!/usr/bin/env bash
# Launcher for the lyx SANDBOX-WEBSTER-SUITE: runs the interactive webster
# black-box agent session (live tmux and a logged-in claude required). The
# machine-specific parent directory is hardcoded HERE (the base under which
# sandbox Hubs are created, mirroring the win/ launcher's C:\Code) -- the Go
# tool stays general.
# Runs in a subshell cd'd to the repo root (two levels up from this sandbox/posix
# folder, mirroring the win/ launcher's pushd/popd) so `go run` finds go.mod without
# touching the caller's own cwd, even if this script is sourced rather than executed.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
(cd "$REPO_ROOT" && go run ./tools/sandbox -parent "$HOME/Code" webster-suite "$@")
exit $?
