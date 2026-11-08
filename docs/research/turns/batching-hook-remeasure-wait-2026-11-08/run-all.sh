#!/usr/bin/env bash
# Usage: run-all.sh [out-dir]
# Rebuilds every table of the report from the Claude Code transcripts on this machine, for the measured sample and the comparison sample:
# <out-dir>/<sample>/turns.tsv, calls.tsv, sessions.tsv, role-summary.tsv, events.tsv, breaches.tsv, breach-summary.tsv,
# edit-chains.tsv, edit-chains-summary.tsv, latency.tsv, latency-summary.tsv and nudge-effect.tsv.
# The samples are new (slugs.txt, the runs after the hook's transcript wait) and before (before-slugs.txt, the runs of the earlier report).
# The extractors and table scripts are those of docs/research/turns/batching-hook-remeasure-2026-11-05, run unchanged.
# Environment: PROJECTS names the transcript root (default ~/.claude/projects), SCRATCH the work directory (default <repo root>/.scratch/batching-hook-remeasure-wait),
# NEW_CUTOFF and BEFORE_CUTOFF the first session timestamp of each sample (default 2026-11-07T13:15:00 and 2026-11-05).
# The default new cutoff is the minute after the wait's commit 7c77e08d, 2026-11-07 14:14:03+01:00.
# Needs bash, jq, awk and git; reads only, apart from the tables and the work directory.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
out=${1:-$here}
scratch=${SCRATCH:-$root/.scratch/batching-hook-remeasure-wait}
earlier=$here/../batching-hook-remeasure-2026-11-05
audit=$here/../turn-audit-2026-10-28
mkdir -p "$out" "$scratch"

# measure <sample> <slugs file> <recipes file> <cutoff>: extract the sample's turns, calls and events, and derive the tables from them.
measure() {
  local sample=$1 slugs=$2 recipes=$3 cutoff=$4
  local stage=$scratch/$sample dest=$out/$sample
  mkdir -p "$stage" "$dest"
  # The extractors read slugs.txt beside themselves, so run copies of them next to this sample's slugs.
  cp "$audit/extract-turns.sh" "$earlier/events.sh" "$earlier/events.jq" "$here/latency.sh" "$stage/"
  cp "$slugs" "$stage/slugs.txt"
  CUTOFF=$cutoff "$stage/extract-turns.sh" "$dest"
  CUTOFF=$cutoff "$stage/events.sh" "$dest"
  CUTOFF=$cutoff "$stage/latency.sh" "$dest"
  "$audit/session-metrics.sh" "$dest" > "$dest/sessions.tsv"
  "$audit/role-summary.sh" "$dest/sessions.tsv" > "$dest/role-summary.tsv"
  "$earlier/breaches.sh" "$dest" > "$dest/breaches.tsv"
  "$earlier/summary.sh" "$dest/breaches.tsv" "$recipes" > "$dest/breach-summary.tsv"
  "$earlier/edit-chains.sh" "$dest" > "$dest/edit-chains.tsv"
  "$earlier/edit-chains-summary.sh" "$dest/edit-chains.tsv" > "$dest/edit-chains-summary.tsv"
  "$here/latency-summary.sh" "$dest/latency.tsv" > "$dest/latency-summary.tsv"
  "$here/nudge-effect.sh" "$dest" > "$dest/nudge-effect.tsv"
}

measure new "$here/slugs.txt" "$here/recipes.tsv" "${NEW_CUTOFF:-2026-11-07T13:15:00}"
measure before "$earlier/slugs.txt" "$earlier/recipes.tsv" "${BEFORE_CUTOFF:-2026-11-05}"
