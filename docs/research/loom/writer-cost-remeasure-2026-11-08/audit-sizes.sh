#!/usr/bin/env bash
# Usage: audit-sizes.sh [out-dir]
# Builds <out-dir>/audit-sizes.tsv: plan cards and changed lines of the runs of the 2026-10-24 plan writer audit,
# joined with that audit's committed costs (q1-table.tsv, kept unchanged), so its table can be normalised by task size.
# Columns: slug, cards, changed_lines, plan_phase_usd, writer_usd, writer_forks_usd, writer_plus_forks_usd, rounds (all from the audit's q1-table.tsv).
# cards and changed_lines are read as plan-table.sh reads them. A run with no archive tag or no landing commit has empty size columns.
# Environment: WEFT names the records repo clone (default <repo root>/../loomyard-weft).
# Needs bash, git and awk; reads only, apart from the table.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
out=${1:-$here}
weft=${WEFT:-$root/../loomyard-weft}
audit=$here/../plan-writer-audit-2026-10-24/q1-table.tsv

printf 'slug\tcards\tchanged_lines\tplan_phase_usd\twriter_usd\twriter_forks_usd\twriter_plus_forks_usd\trounds\n' > "$out/audit-sizes.tsv"
# q1-table.tsv columns: start run profile_note plan_total writer forks writer_plus_forks fork_sessions entries elapsed active rounds
# Only the runs seeded 2026-10-19 or later are kept: the audit's full-fan and five-lens groups, plus the one run that ran across the cut.
awk -F'\t' -v OFS='|' '{ d = $1; sub(/^~/, "", d) } d ~ /^2026/ && d >= "2026-10-19" { print $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12 }' "$audit" | while IFS='|' read -r start slug note plan writer forks both sessions entries elapsed active rounds; do
  tag=$(git -C "$weft" tag -l "archive/$slug/*" --sort=-creatordate | head -1)
  cards=; lines=
  if [ -n "$tag" ]; then
    cards=$(git -C "$weft" ls-tree -r --name-only "$tag" | grep -cE '^_ly[mx]/plan/[0-9]+-' || true)
    title=$( (git -C "$weft" show "$tag:_lym/landing/summary.md" 2> /dev/null || git -C "$weft" show "$tag:_lyx/landing/summary.md" 2> /dev/null) | head -1 | sed 's/^# //' || true)
    commit=
    [ -z "$title" ] || commit=$(git -C "$root" log main --format=%h --fixed-strings --grep="$title" | head -1 || true)
    [ -z "$commit" ] || lines=$(git -C "$root" diff --shortstat "$commit^1" "$commit" | awk '{ print $4 + $6 }' || true)
  fi
  # Without a landing commit the newest tag is not the audited run, so its card count is dropped too.
  [ -n "$lines" ] || cards=
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$cards" "$lines" "$plan" "$writer" "$forks" "$both" "$rounds" >> "$out/audit-sizes.tsv"
done
