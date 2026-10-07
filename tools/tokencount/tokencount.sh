#!/usr/bin/env bash
# Launcher for the token count: writes the markdown report to .scratch/token-usage-by-role.md
# and prints where it landed.
# One fixed path, overwritten every run rather than accumulating a timestamped file per
# count -- the run's own date and time and the counted slugs live inside the report.
# Runs in a subshell cd'd to the repo root (two levels up from this tools/tokencount
# folder, mirroring the .cmd launcher's pushd/popd) so `go run` finds go.mod and -hub
# defaults to the repo root's parent, the hub, without touching the caller's own cwd.
# The arguments are the task slugs to count, and flags such as -hub pass through:
# tokencount.sh bugfix-sessions bugfix-webster
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
(cd "$REPO_ROOT" && go run ./tools/tokencount -out .scratch/token-usage-by-role.md "$@")
exit $?
