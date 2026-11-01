#!/usr/bin/env bash
# Usage: run-all.sh [out-dir]
# Rebuilds every table of the report from the Claude Code transcripts on this machine and the records repo:
# turns.tsv, calls.tsv, sessions.tsv, prices.tsv, sizes.tsv, writers.tsv, groups.tsv.
# out-dir defaults to this directory.
# Environment: PROJECTS names the transcript root (default ~/.claude/projects), WEFT the records repo clone
# (default <repo root>/../loomyard-weft), SCRATCH the work directory (default <repo root>/.scratch/batching-effect-audit).
# Needs bash, git, jq and awk; reads only.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${1:-$here}
scratch=${SCRATCH:-$root/.scratch/batching-effect-audit}
audit=$here/../turn-audit-2026-10-28
mkdir -p "$out" "$scratch/extract"

# The turn audit's extractor reads slugs.txt beside itself, so run a copy of it next to this run's slugs.
cp "$audit/extract-turns.sh" "$scratch/extract/extract-turns.sh"
cp "$here/slugs.txt" "$scratch/extract/slugs.txt"
CUTOFF=2026-10-27 "$scratch/extract/extract-turns.sh" "$scratch/extract/all"

# Keep the Plan and Discussion writers and their forks.
for table in turns calls; do
  awk -F'\t' -v OFS='\t' 'NR == 1 || $4 ~ /^(plan|discussion)(\+fork)?$/' "$scratch/extract/all/$table.tsv" > "$out/$table.tsv"
done

"$audit/session-metrics.sh" "$out" > "$out/sessions.tsv"
"$here/list-price.sh" > "$out/prices.tsv"
"$here/task-size.sh" > "$out/sizes.tsv"
"$here/writers-table.sh" "$out" > "$out/writers.tsv"
"$here/group-means.sh" "$out/writers.tsv" > "$out/groups.tsv"
