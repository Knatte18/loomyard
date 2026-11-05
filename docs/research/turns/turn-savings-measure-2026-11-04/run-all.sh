#!/usr/bin/env bash
# Usage: run-all.sh [out-dir]
# Rebuilds every table of the report from the Claude Code transcripts on this machine, for the measured sample and the baseline sample:
# <out-dir>/<sample>/turns.tsv, calls.tsv, sessions.tsv, role-summary.tsv, events.tsv, breaches.tsv, breach-summary.tsv, gate-loops.tsv, gate-loops-upper.tsv,
# darn-writers.tsv (the measured sample only), and forkaudit.md and tokencount.md for the tapestry runs of the sample.
# The measured sample's review roles go to <out-dir>/new/review/, by the review turn audit's scripts.
# out-dir defaults to this directory; the samples are new (slugs.txt) and baseline (baseline-slugs.txt).
# Environment: PROJECTS names the transcript root (default ~/.claude/projects), SCRATCH the work directory (default <repo root>/.scratch/turn-savings-measure),
# NEW_CUTOFF and BASELINE_CUTOFF the first session date of each sample (default 2026-11-02 and 2026-10-27).
# Needs bash, jq, awk, git and go; reads only, apart from the tables and the work directory.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
out=${1:-$here}
scratch=${SCRATCH:-$root/.scratch/turn-savings-measure}
audit=$here/../turn-audit-2026-10-28
review=$here/../review-turn-audit-2026-11-01
mkdir -p "$out" "$scratch"

# measure <sample> <slugs file> <recipes file> <cutoff>: extract the sample's turns, calls and events, and derive the tables from them.
measure() {
  local sample=$1 slugs=$2 recipes=$3 cutoff=$4
  local stage=$scratch/$sample dest=$out/$sample
  mkdir -p "$stage" "$dest"
  # The extractors read slugs.txt beside themselves, so run copies of them next to this sample's slugs.
  cp "$audit/extract-turns.sh" "$here/events.sh" "$here/events.jq" "$stage/"
  cp "$slugs" "$stage/slugs.txt"
  CUTOFF=$cutoff "$stage/extract-turns.sh" "$dest"
  CUTOFF=$cutoff "$stage/events.sh" "$dest"
  "$audit/session-metrics.sh" "$dest" > "$dest/sessions.tsv"
  "$audit/role-summary.sh" "$dest/sessions.tsv" > "$dest/role-summary.tsv"
  "$here/breaches.sh" "$dest" > "$dest/breaches.tsv"
  "$here/summary.sh" "$dest/breaches.tsv" "$recipes" > "$dest/breach-summary.tsv"
  "$here/gate-loops.sh" "$dest" > "$dest/gate-loops.tsv"
  PENDING_AS_FAIL=1 "$here/gate-loops.sh" "$dest" > "$dest/gate-loops-upper.tsv"
  "$here/darn-writers.sh" "$dest" > "$dest/darn-writers.tsv"
}

measure new "$here/slugs.txt" "$here/recipes.tsv" "${NEW_CUTOFF:-2026-11-02}"
measure baseline "$here/baseline-slugs.txt" "$here/baseline-recipes.tsv" "${BASELINE_CUTOFF:-2026-10-27}"

# Webster forks, by the fork audit and the token count, for the tapestry runs of each sample.
tapestry_runs() { awk -F'\t' 'NR > 1 && $2 == "tapestry" { printf "%s%s", sep, $1; sep = "," }' "$1"; }
for sample in new baseline; do
  recipes=$here/recipes.tsv
  [ "$sample" = baseline ] && recipes=$here/baseline-recipes.tsv
  runs=$(tapestry_runs "$recipes")
  "$root/tools/forkaudit/forkaudit.sh" --runs "$runs" --out "$scratch/$sample/forkaudit.md" --work "$scratch/$sample/forkaudit-work" > /dev/null
  cp "$scratch/$sample/forkaudit.md" "$out/$sample/forkaudit.md"
  (cd "$root" && go run ./tools/tokencount -detail -out "$out/$sample/tokencount.md" ${runs//,/ } > /dev/null)
done

# The review roles of the measured sample, by the review turn audit's scripts run on a staged copy with this sample's slugs.
stage=$scratch/review-stage
mkdir -p "$stage" "$out/new/review"
cp -r "$audit" "$review" "$stage/"
awk -F'\t' 'NR > 1 && $2 == "tapestry" { print $1 }' "$here/recipes.tsv" > "$stage/review-turn-audit-2026-11-01/slugs.txt"
SCRATCH=$stage/scratch "$stage/review-turn-audit-2026-11-01/run-all.sh" "$out/new/review"
