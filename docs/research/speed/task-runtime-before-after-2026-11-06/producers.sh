#!/usr/bin/env bash
# Usage: producers.sh [runs.tsv] > producers.tsv
# Prints, per landed run of runs.tsv, the producers (recipe rows) its shed status records at its archive tag, as "slug<TAB>tag<TAB>start<TAB>recipe<TAB>comma-joined producers in order, a repeat kept".
# A run whose archive tag holds no status file gets "-" for its producers.
# The recipe rows a run went through show which rows the recipe held when it ran, so a row added or dropped inside the window shows as a change in the share of runs holding it.
# Needs bash, jq and git; reads only. WEFT names the weft repository (default: the prime's, with -weft appended).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
runs=${1:-$here/runs.tsv}
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
weft=${WEFT:-$prime-weft}

awk -F'\t' -v OFS='\t' 'NR > 1 { print $1, $2, $4, $3 }' "$runs" |
  while IFS=$'\t' read -r slug tag start recipe; do
    producers=$(git -C "$weft" show "$tag:_lym/shed/$slug/status.json" 2>/dev/null | jq -r '[.history[].producer] | join(",")' 2>/dev/null || true)
    printf '%s\t%s\t%s\t%s\t%s\n' "$slug" "$tag" "$start" "$recipe" "${producers:--}"
  done
