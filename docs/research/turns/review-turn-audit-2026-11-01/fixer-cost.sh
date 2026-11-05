#!/usr/bin/env bash
# Usage: fixer-cost.sh <session-costs.tsv> > fixer-cost.tsv
# What the Discussion and Plan fixers cost at list price as run, and with every message priced at the sonnet-5-5 row instead:
# slug role sessions forks actual_usd sonnet_usd saving_usd saving_share
# A fixer's cost is its main sessions plus its fork sub-agents; the forks run on the fixer's model.
# sonnet_usd keeps the fixer's turns and context as they were: a Sonnet fixer that needs more turns, or that leaves findings
# an Opus fixer would have closed, costs more than this figure, and the figure does not price those rounds.
# A final "all" row per role sums the runs.
set -euo pipefail
awk -F'\t' -v OFS='\t' '
NR == 1 { next }          # session-costs.tsv: slug session parent kind role round model start end wall_s turns ctx_mean ctx_peak cost_usd cost_sonnet_usd
{
  role = $5; sub(/\+fork$/, "", role)
  if (role !~ /-burler-fix$/) next
  k = $1 SUBSEP role
  if (!(k in seen)) { seen[k] = 1; keys[++n] = k }
  if ($4 == "main") sessions[k]++; else forks[k]++
  actual[k] += $14; sonnet[k] += $15
}
END {
  print "slug", "role", "sessions", "forks", "actual_usd", "sonnet_usd", "saving_usd", "saving_share"
  for (i = 1; i <= n; i++) {
    split(keys[i], p, SUBSEP); k = keys[i]
    printf "%s\t%s\t%d\t%d\t%.2f\t%.2f\t%.2f\t%.3f\n", p[1], p[2], sessions[k], forks[k] + 0, actual[k], sonnet[k], actual[k] - sonnet[k], (actual[k] - sonnet[k]) / actual[k]
    r = p[2]; tsess[r] += sessions[k]; tfork[r] += forks[k]; ta[r] += actual[k]; ts[r] += sonnet[k]
  }
  for (r in tsess) printf "all\t%s\t%d\t%d\t%.2f\t%.2f\t%.2f\t%.3f\n", r, tsess[r], tfork[r], ta[r], ts[r], ta[r] - ts[r], (ta[r] - ts[r]) / ta[r]
}' "$1" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k2,2 -k1,1; }
