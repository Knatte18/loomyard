#!/usr/bin/env bash
# Usage: writers-table.sh <dir-with-turns-sessions-prices-sizes> > writers.tsv
# One row per Plan or Discussion writer main session, joining sessions.tsv, turns.tsv, prices.tsv and sizes.tsv:
# slug role batching landed turns calls_per_turn single_share serial_strict serial_class serial_share ctx_mean ctx_peak
#   cost_usd forks fork_turns fork_cost_usd total_cost_usd decision_record_bytes plan_cards plan_bytes cost_per_size_kb
# serial_strict and serial_class are the turn audit's mergeable-turn counts (session-metrics.sh m_saved_turns and u_saved_turns):
# single-call read, search, git-read, lym-read, quarry or skill turns after the first of a run of them;
# strict leaves out a turn whose call needs a path out of the previous result, class is the upper bound.
# serial_share is serial_strict over turns.
# ctx is a turn's input + cache write + cache read tokens; ctx_mean is the mean over the session's turns.
# landed is "yes" when the run has an archive tag in runs.tsv.
# fork_* sum the writer's fork sub-agents (the twill lens forks and the plan's whole-plan fork).
# cost_per_size_kb is cost_usd over the writer's input size in KB: the plan's cards for a Plan writer, the decision record for a Discussion writer.
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' '
FILENAME == ARGV[1] {   # prices.tsv: slug session kind role batching cost_usd
  if (FNR == 1) next
  if ($3 == "main") { batching[$1, $4] = $5; cost[$1, $2] = $6 }
  else { base = $4; sub(/\+fork$/, "", base); fcost[$1, base] += $6 }
  next
}
FILENAME == ARGV[2] {   # sizes.tsv: slug archive_tag decision_record_bytes plan_cards plan_bytes
  if (FNR == 1) next
  landed[$1] = ($2 == "-") ? "no" : "yes"
  record[$1] = $3; cards[$1] = $4; planbytes[$1] = $5
  next
}
FILENAME == ARGV[3] {   # turns.tsv: slug session kind role turn ts ncalls ctx ...
  if (FNR == 1) next
  key = $1 SUBSEP $2
  tn[key]++
  csum[key] += $8
  if ($8 > cpeak[key]) cpeak[key] = $8
  next
}
{                       # sessions.tsv: slug session kind role turns calls single zero weight m_runs m_turns m_saved_turns m_weight m_saved_weight u_runs u_turns u_saved_turns ...
  if (FNR == 1) next
  if ($3 == "fork") {
    base = $4; sub(/\+fork$/, "", base)
    forks[$1, base]++; fturns[$1, base] += $5
    next
  }
  rows[++n] = $0
}
END {
  print "slug", "role", "batching", "landed", "turns", "calls_per_turn", "single_share", "serial_strict", "serial_class", "serial_share",
    "ctx_mean", "ctx_peak", "cost_usd", "forks", "fork_turns", "fork_cost_usd", "total_cost_usd",
    "decision_record_bytes", "plan_cards", "plan_bytes", "cost_per_size_kb"
  for (i = 1; i <= n; i++) {
    split(rows[i], f, "\t")
    slug = f[1]; sid = f[2]; role = f[4]; key = slug SUBSEP sid
    c = cost[slug, sid] + 0; fc = fcost[slug, role] + 0
    kb = ((role == "plan") ? planbytes[slug] : record[slug]) / 1000
    printf "%s\t%s\t%s\t%s\t%d\t%.2f\t%.2f\t%d\t%d\t%.2f\t%d\t%d\t%.2f\t%d\t%d\t%.2f\t%.2f\t%d\t%d\t%d\t%.3f\n",
      slug, role, batching[slug, role], landed[slug], f[5], f[6] / f[5], f[7] / f[5], f[12], f[17], f[12] / f[5],
      csum[key] / tn[key], cpeak[key], c, forks[slug, role], fturns[slug, role], fc, c + fc,
      record[slug], cards[slug], planbytes[slug], c / kb
  }
}' "$dir/prices.tsv" "$dir/sizes.tsv" "$dir/turns.tsv" "$dir/sessions.tsv" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k2,2 -k3,3r -k1,1; }
