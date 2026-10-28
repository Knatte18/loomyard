#!/usr/bin/env bash
# Usage: kinds-total.sh <merge-kinds.tsv> > kinds-total.tsv
# Sums merge-kinds.tsv over roles: class command removable_turns removable_weight
set -euo pipefail
awk -F'\t' -v OFS='\t' '
NR == 1 { next }
{ k = $2 OFS $3; n[k] += $4; w[k] += $5 }
END { print "class", "command", "removable_turns", "removable_weight"; for (k in n) print k, n[k], w[k] }
' "$1" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k3,3nr; }
