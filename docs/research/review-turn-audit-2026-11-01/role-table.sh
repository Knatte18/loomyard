#!/usr/bin/env bash
# Usage: role-table.sh <dir-with-session-costs> > role-table.tsv
# Per role, from session-costs.tsv:
# role sessions turns turns_per_session ctx_first ctx_mean ctx_peak cost_usd cost_per_session read_share write_share output_share wall_s wall_s_per_session
# A reviewer or fixer session that ran on Sonnet keeps its own row, its role followed by "@sonnet", as in turns.tsv.
# ctx_first is the mean over sessions of the first turn's context, ctx_mean the mean context per turn over all the role's turns, ctx_peak the largest context of any turn.
# cost_usd is the list-price cost of the role's sessions; a +fork role sums its forks, and a main role leaves its forks out.
# read_share, write_share and output_share are the cache-read, cache-write and output parts of cost_usd; the rest is uncached input.
# wall_s sums each session's seconds from first to last message; forks of one parent run side by side, so a +fork row's sum is not elapsed time.
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' '
NR == 1 { next }         # slug session parent kind role round model start end wall_s turns ctx_mean ctx_peak cost_usd cost_sonnet_usd ctx_first read_usd write_usd output_usd
{
  r = $5
  if ($7 == "claude-sonnet-5-5" && r ~ /burler-(review|fix)/) sub(/(\+fork)?$/, "@sonnet&", r)
  n[r]++; t[r] += $11; cs[r] += $12 * $11; c[r] += $14; w[r] += $10; f[r] += $16
  rd[r] += $17; wr[r] += $18; ou[r] += $19
  if ($13 > pk[r]) pk[r] = $13
}
END {
  print "role", "sessions", "turns", "turns_per_session", "ctx_first", "ctx_mean", "ctx_peak", "cost_usd", "cost_per_session", "read_share", "write_share", "output_share", "wall_s", "wall_s_per_session"
  for (r in n) printf "%s\t%d\t%d\t%.1f\t%d\t%d\t%d\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%d\t%d\n", r, n[r], t[r], t[r] / n[r], f[r] / n[r], cs[r] / t[r], pk[r], c[r], c[r] / n[r], rd[r] / c[r], wr[r] / c[r], ou[r] / c[r], w[r], w[r] / n[r]
}' "$dir/session-costs.tsv" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k1,1; }
