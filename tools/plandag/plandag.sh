#!/usr/bin/env bash
# Launcher for the plan card graph: writes plandag.md, plandag.dot and, with dot on PATH,
# plandag.svg into .scratch/plandag and prints where each landed.
# Fixed file names, overwritten every run; the report names the plan it drew.
# The one argument is the plan directory, made absolute here because the tool runs in a
# subshell cd'd to the repo root (two levels up from this tools/plandag folder, mirroring the
# .cmd launcher's pushd/popd) so `go run` finds go.mod:
# plandag.sh ../some-pair/_lyx/plan
if [ "$#" -ne 1 ]; then
	echo "usage: plandag.sh <plan-dir>" >&2
	exit 2
fi
PLAN_DIR="$(cd "$1" && pwd)" || exit 1
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
(cd "$REPO_ROOT" && go run ./tools/plandag -out .scratch/plandag "$PLAN_DIR")
exit $?
