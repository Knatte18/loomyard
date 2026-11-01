#!/usr/bin/env bash
# Usage: group-means.sh <writers.tsv> > groups.tsv
# Means of every numeric column of writers.tsv (column 5 on) per role and batching group, over the landed runs only,
# after a sessions column: role batching sessions <means...>
# A run that has not landed is left out; the report lists it beside the groups.
set -euo pipefail
awk -F'\t' -v OFS='\t' '
NR == 1 { cols = NF; for (i = 5; i <= NF; i++) name[i] = $i; next }
$4 != "yes" { next }
{
  g = $2 OFS $3; n[g]++
  for (i = 5; i <= NF; i++) sum[g, i] += $i
}
END {
  line = "role" OFS "batching" OFS "sessions"
  for (i = 5; i <= cols; i++) line = line OFS name[i]
  print line
  for (g in n) {
    line = g OFS n[g]
    for (i = 5; i <= cols; i++) line = line OFS sprintf("%.2f", sum[g, i] / n[g])
    print line
  }
}' "$1" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k1,1 -k2,2r; }
