#!/usr/bin/env bash
# Usage: edit-chains.sh <dir-with-turns-calls-and-events> > edit-chains.tsv
# One row per edit chain of a session that holds an edit follow, from turns.tsv and calls.tsv (extract-turns.sh of the turn audit) and events.tsv (events.sh):
# slug session kind role start_turn chain_turns follow_turns follow_ctx origin
# A chain is a run of consecutive turns that each hold only Edit and Write calls; a follow is a turn of the chain after its first that has context at or above CONTEXT_TOKENS (default 150000),
# the turns breaches.sh counts as ef_turns, so follow_turns summed over the rows equals ef_turns summed over breaches.tsv.
# origin says what the session did before the chain began, by the last gate or check result before the chain's first turn:
#   after-gate-fail: the last result is a failed `lym gate test`, so the chain is the fix of a failed gate;
#   after-check-fail: the last result is a failed validator or lint call (events.sh type check), the fix of a gate-less round's validator;
#   mid-round: any other state, a green or pending result or no result yet, so the chain splits what a round of edits could make in one message.
# A call whose verdict the transcript does not hold is pending, which counts as mid-round and so makes the two after-fail counts lower bounds.
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' -v line="${CONTEXT_TOKENS:-150000}" '
function origin_of(sid, turn,    n, last) {
  for (n = 1; n <= nres[sid]; n++) if (rturn[sid, n] < turn) last = n
  if (last == "" || rres[sid, last] != "fail") return "mid-round"
  return rtype[sid, last] == "gate" ? "after-gate-fail" : "after-check-fail"
}
function flush() {
  if (chainlen >= 2 && fol > 0) print slug, session, kind, role, start, chainlen, fol, folctx, origin_of(sid, start)
  chainlen = fol = folctx = 0
}
FNR == 1 { next }
FILENAME == ARGV[1] {
  # calls.tsv: slug session kind role turn call ts tool class key
  if ($8 == "Edit" || $8 == "Write") edits[$1 SUBSEP $2 SUBSEP $5]++
  next
}
FILENAME == ARGV[2] {
  # events.tsv: slug session kind role type turn a b c
  if ($5 != "gate" && $5 != "check") next
  g = $1 SUBSEP $2
  n = ++nres[g]; rturn[g, n] = $6; rres[g, n] = $8; rtype[g, n] = $5
  next
}
{
  # turns.tsv: slug session kind role turn ts ncalls ctx input cache_write cache_read output weight dep
  s = $1 SUBSEP $2
  if (s != sid) { flush(); sid = s; slug = $1; session = $2; kind = $3; role = $4 }
  isedit = ($7 >= 1 && edits[$1 SUBSEP $2 SUBSEP $5] == $7)
  if (!isedit) { flush(); next }
  if (chainlen == 0) start = $5
  chainlen++
  if (chainlen >= 2 && $8 >= line) { fol++; folctx += $8 }
}
END { flush() }
' "$dir/calls.tsv" "$dir/events.tsv" "$dir/turns.tsv" | awk 'BEGIN { OFS = "\t"; print "slug","session","kind","role","start_turn","chain_turns","follow_turns","follow_ctx","origin" } { print }'
