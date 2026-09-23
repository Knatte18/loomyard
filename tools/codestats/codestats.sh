#!/usr/bin/env bash
# Launcher for the codestats scan: writes the markdown report to .scratch/codestats.md
# and prints where it landed.
# One fixed path, overwritten every run rather than accumulating a timestamped file per
# scan -- the run's own date and time live inside the report, under Generated, which is
# what keeps a copied or pasted report self-describing without littering .scratch.
# Runs in a subshell cd'd to the repo root (two levels up from this tools/codestats
# folder, mirroring the .cmd launcher's pushd/popd) so `go run` finds go.mod without
# touching the caller's own cwd, even if this script is sourced rather than executed.
# Any extra arguments are passed through to the tool, so -root, -exclude and -hidden
# still work: codestats.sh -root ../Models
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
(cd "$REPO_ROOT" && go run ./tools/codestats -out .scratch/codestats.md "$@")
exit $?
