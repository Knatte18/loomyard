#!/usr/bin/env bash
# Usage: session-table.sh <label> <dir-with-writer-tables> >> session-table.tsv
# One row per Plan-Write and Discussion-Write session of one sample, from writer-turns.tsv, writer-subagents.tsv and writer-sessions.tsv:
#   sample slug role turns lookups first_lookup_turn ctx_at_first_lookup explore_turns explore_before explore_after chains explore_cost_usd session_cost_usd
# lookups counts the session's lookup subagents (kind lookup in writer-subagents.tsv); first_lookup_turn is the turn that started the first lookup batch,
# and ctx_at_first_lookup the context of that turn. Both are empty when the session started no lookup subagent.
# explore_before and explore_after count the exploration turns before and after that turn; a session with no lookup batch has all its exploration before.
set -euo pipefail
label=$1
dir=$2
awk -F'\t' -v OFS='\t' -v label="$label" '
FILENAME == ARGV[1] {
  if (FNR == 1) next
  if ($5 == "lookup") { lookups[$1 SUBSEP $2]++; lookupbatch[$1 SUBSEP $2 SUBSEP $4] = 1 }
  next
}
FILENAME == ARGV[2] {
  if (FNR == 1 || $1 == "TOTAL") next
  chains[$1 SUBSEP $2] = $10
  next
}
FNR == 1 { next }
{
  sid = $1 SUBSEP $2
  if (sid != cur) { flush(); cur = sid; slug = $1; session = $2; role = $3; n = 0; batch = 0; first = ""; fctx = ""; before = after = 0; ecost = 0; cost = 0 }
  n++; cost += $6
  if ($8 == "delegate") { batch++; if ((sid SUBSEP batch) in lookupbatch && first == "") { first = n; fctx = $5 } }
  if ($8 == "explore") { ecost += $6; if (first == "") before++; else after++ }
}
function flush() {
  if (cur == "") return
  printf "%s\t%s\t%s\t%d\t%d\t%s\t%s\t%d\t%d\t%d\t%d\t%.2f\t%.2f\n", label, slug, role, n, lookups[cur], first, fctx, before + after, before, after, chains[cur], ecost, cost
}
END { flush() }
' "$dir/writer-subagents.tsv" "$dir/writer-sessions.tsv" "$dir/writer-turns.tsv"
