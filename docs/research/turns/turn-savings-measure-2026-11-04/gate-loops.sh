#!/usr/bin/env bash
# Usage: gate-loops.sh <dir-with-turns-and-events> > gate-loops.tsv
# One row per card gate fix loop of a session, from turns.tsv (extract-turns.sh of the turn audit) and events.tsv (events.sh):
# slug session role fail_turn green_turn turns ctx_tokens weight peak_ctx
# A loop opens at a `lym gate test --gate card` call whose result is fail and closes at the next card gate call whose result is green in the same session.
# turns counts the turns after the failing call's turn up to and including the green call's turn (the fixes and the green re-run);
# ctx_tokens sums those turns' input + cache write + cache read, weight their input + 1.25 x cache write + 0.1 x cache read + 5 x output, and peak_ctx is the largest context among them.
# A card gate call whose result is pending, because it moved to the background, closes nothing and opens a loop only when PENDING_AS_FAIL is 1,
# which gives the upper bound: a backgrounded gate's verdict arrives outside the tool result, so the transcript does not hold it here.
# A loop still open at a session's end is listed with an empty green_turn and ends at the last turn.
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' -v pendingfails="${PENDING_AS_FAIL:-0}" '
function emit(sid, ft, gt, role,    p, t, e, tok, wt, pk) {
  e = (gt == "" ? last[sid] : gt)
  for (t = ft + 1; t <= e; t++) { tok += ctx[sid, t]; wt += wgt[sid, t]; if (ctx[sid, t] > pk) pk = ctx[sid, t] }
  split(sid, p, SUBSEP)
  print p[1], p[2], role, ft, gt, e - ft, tok, int(wt), pk
}
FNR == 1 { next }
FILENAME == ARGV[1] {
  # events.tsv: slug session kind role type turn a b c
  if ($5 != "gate" || $7 != "card") next
  sid = $1 SUBSEP $2
  if (!(sid in seen)) { seen[sid] = 1; order[++count] = sid }
  role[sid] = $4
  nev[sid]++
  evturn[sid, nev[sid]] = $6; evres[sid, nev[sid]] = $8
  next
}
{
  # turns.tsv: slug session kind role turn ts ncalls ctx input cache_write cache_read output weight dep
  sid = $1 SUBSEP $2
  ctx[sid, $5] = $8; wgt[sid, $5] = $13
  if ($5 > last[sid]) last[sid] = $5
}
END {
  print "slug", "session", "role", "fail_turn", "green_turn", "turns", "ctx_tokens", "weight", "peak_ctx"
  for (i = 1; i <= count; i++) {
    sid = order[i]; open = ""
    for (e = 1; e <= nev[sid]; e++) {
      failed = (evres[sid, e] == "fail" || (pendingfails == 1 && evres[sid, e] == "pending"))
      if (failed && open == "") open = evturn[sid, e]
      else if (evres[sid, e] == "green" && open != "") { emit(sid, open, evturn[sid, e], role[sid]); open = "" }
    }
    if (open != "") emit(sid, open, "", role[sid])
  }
}' "$dir/events.tsv" "$dir/turns.tsv"
