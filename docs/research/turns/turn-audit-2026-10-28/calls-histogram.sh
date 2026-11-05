#!/usr/bin/env bash
# Usage: calls-histogram.sh <turns.tsv> > calls-histogram.tsv
# Per role, the turns by tool-call count: role turns zero one two three_plus max
set -euo pipefail
awk -F'\t' -v OFS='\t' '
NR == 1 { next }
{ r = $4; t[r]++; if ($7 == 0) z[r]++; else if ($7 == 1) o[r]++; else if ($7 == 2) d[r]++; else p[r]++; if ($7 > mx[r]) mx[r] = $7 }
END {
  print "role", "turns", "zero", "one", "two", "three_plus", "max"
  for (r in t) print r, t[r], z[r] + 0, o[r] + 0, d[r] + 0, p[r] + 0, mx[r]
}' "$1" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k2,2nr; }
