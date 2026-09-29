#!/usr/bin/env bash
# Put the checked-out, committed code into production: mirror every installed loomyard plugin into
# the Claude Code plugin cache and build lyx into the Go bin dir (`go env GOBIN`, else GOPATH/bin).
# This is the only production route; it refuses a dirty tree. Dev builds go through ./deploy-dev.
# Logic lives in tools/deploy (shared with update-plugins.cmd); cd to the repo root so `go run`
# finds go.mod.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
exec go run ./tools/deploy "$@"
