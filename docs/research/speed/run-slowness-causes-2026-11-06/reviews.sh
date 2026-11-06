#!/usr/bin/env bash
# Usage: reviews.sh [runs.tsv] > reviews.tsv
# Prints one line per review round of every landed run, read from the newest facts file of each review at the run's archive tag:
# slug tag recipe start perch round findings blocking medium low nit gating
# perch is the directory the round sits in under the run's reviews (code, plan or discussion), and gating counts the findings that keep a round from converging.
# A run whose tag holds no facts file prints nothing.
# Needs bash, awk and git; reads only. WEFT names the records repository (default: the prime's, with -weft appended).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
runs=${1:-$here/runs.tsv}
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
weft=${WEFT:-$prime-weft}

printf 'slug\ttag\trecipe\tstart\tperch\tround\tfindings\tblocking\tmedium\tlow\tnit\tgating\n'
awk -F'\t' -v OFS='\t' 'NR > 1 { print $1, $2, $3, $4 }' "$runs" |
  while IFS=$'\t' read -r slug tag recipe start; do
    # A facts file lists every round up to its own, so the newest file of a perch holds them all.
    git -C "$weft" ls-tree -r --name-only "$tag" -- _lym/reviews 2>/dev/null | { grep -E '/round-[0-9]+-facts\.md$' || true; } |
      awk -F/ '{ dir = $0; sub(/\/[^\/]*$/, "", dir); n = $NF; gsub(/[^0-9]/, "", n); if (n + 0 >= best[dir] + 0) { best[dir] = n; path[dir] = $0 } } END { for (dir in path) print path[dir] }' |
      while read -r path; do
        perch=$(basename "$(dirname "$path")")
        git -C "$weft" show "$tag:$path" |
          awk -F'|' -v OFS='\t' -v head="$slug"$'\t'"$tag"$'\t'"$recipe"$'\t'"$start"$'\t'"$perch" '
            /^\| *[0-9]+ *\|/ { for (i = 2; i <= NF; i++) gsub(/ /, "", $i); print head, $2, $3, $4, $5, $6, $7, $13 }'
      done
  done
