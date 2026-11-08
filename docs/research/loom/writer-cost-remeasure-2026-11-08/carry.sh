#!/usr/bin/env bash
# Usage: carry.sh <label> <dir-with-writer-turns-and-writer-subagents> >> carry.tsv
# Prices the carry of lookup results for each Plan-Write and Discussion-Write session of one sample, one row per session.
# Carry is the context a lookup adds, paid in every later turn: one cache write at 5 USD per million tokens in the turn after it,
# then a cache read at 0.2 USD per million in each turn after that (the Opus 5.5 list price, as writers.sh).
# Two sources of lookup context are priced apart:
#   own_*   the results of the writer's own exploration turns (kind explore in writer-turns.tsv);
#   agent_* the replies of lookup subagents, the context growth from the turn that started a lookup batch to the next turn.
#           A batch is a lookup batch when writer-subagents.tsv holds a lookup row for it; a fork batch (the plan-writing fork, the lens reviews) is not.
# first_lookup_turn is the turn that started the first lookup batch; empty when the session started none.
# lookup_cost_usd is the price of the lookup subagents themselves, from writer-subagents.tsv.
# Columns: sample slug session role turns delegated first_lookup_turn own_tokens own_carry_usd agent_tokens agent_carry_usd total_carry_usd lookup_cost_usd session_cost_usd
set -euo pipefail
label=$1
dir=$2
awk -F'\t' -v OFS='\t' -v label="$label" '
FILENAME == ARGV[1] {
  if (FNR == 1) next
  if ($5 == "lookup") { lookupbatch[$1 SUBSEP $2 SUBSEP $4] = 1; lookupcost[$1 SUBSEP $2] += $10 }
  next
}
FNR == 1 { next }
{
  sid = $1 SUBSEP $2
  if (sid != cur) { flush(); cur = sid; slug = $1; session = $2; role = $3; n = 0; batch = 0 }
  n++
  ctx[n] = $5; cost += $6; kind[n] = $8
  if ($8 == "delegate") { batch++; isbatch[n] = (sid SUBSEP batch) in lookupbatch }
  else isbatch[n] = 0
}
function flush(   i, g, c, own, owncarry, ag, agcarry, first, deleg) {
  if (cur == "") return
  for (i = 1; i < n; i++) {
    g = ctx[i + 1] - ctx[i]
    if (g <= 0) continue
    c = g * (5 + 0.2 * (n - (i + 1))) / 1e6
    if (kind[i] == "explore") { own += g; owncarry += c }
    else if (isbatch[i]) { ag += g; agcarry += c }
  }
  for (i = 1; i <= n; i++) if (isbatch[i]) { deleg++; if (first == "") first = i }
  printf "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%d\t%.4f\t%d\t%.4f\t%.4f\t%.4f\t%.4f\n", label, slug, session, role, n, deleg, first, own, owncarry, ag, agcarry, owncarry + agcarry, lookupcost[cur], sessioncost
  delete ctx; delete kind; delete isbatch
  cost = 0
}
{ sessioncost = cost }
END { flush() }
' "$dir/writer-subagents.tsv" "$dir/writer-turns.tsv"
