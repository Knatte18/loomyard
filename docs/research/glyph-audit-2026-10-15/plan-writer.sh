#!/usr/bin/env bash
# Find each run's plan-writing session transcripts (top-level sessions that Write _lyx/plan/NN-*.md cards) and count their tool use.
# Output: plan-writer.tsv  columns: arm, proj, then plan-writer.jq columns
set -u
here="$(cd "$(dirname "$0")" && pwd)"
cd "$HOME/.claude/projects" || exit 1
: > "$here/plan-writer.tsv"
cut -f1,2 "$here/plan-runs.tsv" | sort -u | while IFS=$'\t' read -r arm proj; do
  d="-home-knatte-Code-loomyard-LYXHUB-$proj"
  [ -d "$d" ] || { printf '%s\t%s\tNO_PROJECT_DIR\n' "$arm" "$proj" >> "$here/plan-writer.tsv"; continue; }
  # top-level session files only (not subagents): the directory's own *.jsonl
  for f in $(grep -lE '"file_path":"([^"]*/)?_lyx/plan/[0-9][0-9]-' -- "$d"/*.jsonl 2>/dev/null); do
    row=$(jq -s -r -f "$here/plan-writer.jq" -- "$f")
    printf '%s\t%s\t%s\n' "$arm" "$proj" "$row" >> "$here/plan-writer.tsv"
  done
done
sort -t$'\t' -k1,2 "$here/plan-writer.tsv" | cut -f1-17
