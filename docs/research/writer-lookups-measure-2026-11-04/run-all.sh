#!/usr/bin/env bash
# Usage: run-all.sh [out-dir]
# Rebuilds every table of the report from the Claude Code transcripts on this machine:
# <out-dir>/turns.tsv, calls.tsv (the turn audit's extractor on the Plan-Write and Discussion-Write sessions of the slugs in slugs.txt, with their forks),
# writer-turns.tsv, writer-sessions.tsv, writer-chains.tsv (writers.sh) and writer-subagents.tsv (subagents.sh).
# out-dir defaults to this directory.
# Environment: PROJECTS names the transcript root (default ~/.claude/projects), SCRATCH the work directory (default <repo root>/.scratch/writer-lookups-measure),
# CUTOFF the first session date (default 2026-11-02), LINE_TOKENS the context line of the over figures (default 200000).
# Needs bash, jq and awk; reads only, apart from the tables and the work directory.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${1:-$here}
scratch=${SCRATCH:-$root/.scratch/writer-lookups-measure}
audit=$here/../turn-audit-2026-10-28
mkdir -p "$out" "$scratch"

# The extractor reads slugs.txt beside itself, so run a copy of it next to this sample's slugs.
cp "$audit/extract-turns.sh" "$scratch/"
cp "$here/slugs.txt" "$scratch/slugs.txt"
CUTOFF=${CUTOFF:-2026-11-02} "$scratch/extract-turns.sh" "$scratch/extract"
"$here/writers.sh" "$scratch/extract" "$out"
# Keep only the writers' own rows in the published turns and calls; the rest of the extraction is every role of the three runs.
for table in turns calls; do
  awk -F'\t' 'NR == 1 || ($3 == "main" && ($4 == "plan" || $4 == "discussion"))' "$scratch/extract/$table.tsv" > "$out/$table.tsv"
done
CUTOFF=${CUTOFF:-2026-11-02} "$here/subagents.sh" "$out"
