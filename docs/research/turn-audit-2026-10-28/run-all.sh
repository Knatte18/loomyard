#!/usr/bin/env bash
# Usage: run-all.sh [out-dir]
# Rebuilds every table of the report from the Claude Code transcripts on this machine:
# turns.tsv, calls.tsv, sessions.tsv, role-summary.tsv, merge-kinds.tsv, kinds-total.tsv, calls-histogram.tsv.
# out-dir defaults to this directory.
# Environment: PROJECTS names the transcript root (default ~/.claude/projects), CUTOFF the first session date (default 2026-10-20).
# Needs bash, jq and awk; reads only.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
out=${1:-$here}
"$here/extract-turns.sh" "$out"
"$here/session-metrics.sh" "$out" > "$out/sessions.tsv"
"$here/role-summary.sh" "$out/sessions.tsv" > "$out/role-summary.tsv"
"$here/merge-kinds.sh" "$out" > "$out/merge-kinds.tsv"
"$here/kinds-total.sh" "$out/merge-kinds.tsv" > "$out/kinds-total.tsv"
"$here/calls-histogram.sh" "$out/turns.tsv" > "$out/calls-histogram.tsv"
