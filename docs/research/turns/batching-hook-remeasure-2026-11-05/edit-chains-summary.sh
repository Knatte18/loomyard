#!/usr/bin/env bash
# Usage: edit-chains-summary.sh <edit-chains.tsv> > edit-chains-summary.tsv
# Sums edit-chains.sh rows by session kind (main or fork) and origin, with an "all" row per kind.
# Columns: kind origin chains chain_turns follow_turns follow_ctx
set -euo pipefail
awk -F'\t' -v OFS='\t' '
FNR == 1 { next }
{
  for (k = 0; k < 2; k++) {
    key = $3 SUBSEP (k == 0 ? $9 : "all")
    if (!(key in seen)) { seen[key] = 1; order[++count] = key }
    chains[key]++; turns[key] += $6; follows[key] += $7; ctx[key] += $8
  }
}
END {
  print "kind", "origin", "chains", "chain_turns", "follow_turns", "follow_ctx"
  for (i = 1; i <= count; i++) { split(order[i], p, SUBSEP); print p[1], p[2], chains[order[i]], turns[order[i]], follows[order[i]], ctx[order[i]] }
}' "$1" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k1,1 -k2,2; }
