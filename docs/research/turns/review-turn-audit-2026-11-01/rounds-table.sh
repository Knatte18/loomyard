#!/usr/bin/env bash
# Usage: rounds-table.sh <dir-with-session-costs-and-sessions> > rounds.tsv
# One row per main session of the review segment, joining session-costs.tsv and sessions.tsv:
# slug role round model wall_s turns calls_per_turn single_share ctx_mean ctx_peak serial_strict serial_class
#   forks fork_turns cost_usd fork_cost_usd total_cost_usd
# serial_strict and serial_class are the turn audit's mergeable-turn counts (session-metrics.sh m_saved_turns and u_saved_turns):
# single-call read, search, git-read, lym-read, quarry or skill turns after the first of a run of them;
# strict leaves out a turn whose call needs a path out of the previous result, class is the upper bound.
# turns counts the extractor's turns, which is the session-costs.tsv count for a main session.
# forks and fork_turns count the session's fork sub-agents and their turns; the fork_cost_usd sums their list-price cost.
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' '
FILENAME == ARGV[1] {   # session-costs.tsv: slug session parent kind role round model start end wall_s turns ctx_mean ctx_peak cost_usd cost_sonnet_usd
  if (FNR == 1) next
  if ($4 == "fork") { forks[$3]++; fturns[$3] += $11; fcost[$3] += $14; next }
  slug[$2] = $1; role[$2] = $5; round[$2] = $6; model[$2] = $7; wall[$2] = $10; turns[$2] = $11
  cmean[$2] = $12; cpeak[$2] = $13; cost[$2] = $14
  order[++n] = $2
  next
}
{                       # sessions.tsv: slug session kind role turns calls single zero weight m_runs m_turns m_saved_turns m_weight m_saved_weight u_runs u_turns u_saved_turns ...
  if (FNR == 1 || $3 == "fork") next
  calls[$2] = $6; single[$2] = $7; strict[$2] = $12; upper[$2] = $17
}
END {
  print "slug", "role", "round", "model", "wall_s", "turns", "calls_per_turn", "single_share", "ctx_mean", "ctx_peak", "serial_strict", "serial_class", "forks", "fork_turns", "cost_usd", "fork_cost_usd", "total_cost_usd"
  for (i = 1; i <= n; i++) {
    s = order[i]
    printf "%s\t%s\t%s\t%s\t%d\t%d\t%.2f\t%.2f\t%d\t%d\t%d\t%d\t%d\t%d\t%.2f\t%.2f\t%.2f\n",
      slug[s], role[s], round[s], model[s], wall[s], turns[s], calls[s] / turns[s], single[s] / turns[s], cmean[s], cpeak[s],
      strict[s], upper[s], forks[s] + 0, fturns[s] + 0, cost[s], fcost[s] + 0, cost[s] + fcost[s]
  }
}' "$dir/session-costs.tsv" "$dir/sessions.tsv" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k2,2 -k1,1 -k3,3n; }
