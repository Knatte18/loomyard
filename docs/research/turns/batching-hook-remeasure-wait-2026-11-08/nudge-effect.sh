#!/usr/bin/env bash
# Usage: nudge-effect.sh <dir-with-turns-calls-and-events> > nudge-effect.tsv
# Compares what the next message does after a breach turn the hook nudged with what it does after one it left silent.
# A breach turn is a turn at or above CONTEXT_TOKENS (default 150000, the hook's context_tokens) that ends a run of
# 3 or more messages that each hold one call the turn audit classes read or search (lookup), or a run of 2 or more
# messages that hold only Edit and Write calls (edit): the hook's lookup_run and edit_run.
# A nudged turn is one with a nudge row of the same breach at the same turn in events.tsv (events.sh).
# The next message is batched when it holds 2 or more calls; a session's last turn has no next message and is left out.
# Columns: breach nudged turns next_batched next_batched_share
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' -v line="${CONTEXT_TOKENS:-150000}" '
function resolve(ncalls,    b) {
  for (b = 1; b <= 2; b++) {
    if (pendinggroup[b] == "") continue
    turns[pendinggroup[b]]++
    if (ncalls >= 2) batched[pendinggroup[b]]++
    pendinggroup[b] = ""
  }
}
FNR == 1 { next }
FILENAME == ARGV[1] {
  # events.tsv: slug session kind role type turn a b c
  if ($5 == "nudge") nudged[$1 SUBSEP $2 SUBSEP $6 SUBSEP $7] = 1
  next
}
FILENAME == ARGV[2] {
  # calls.tsv: slug session kind role turn call ts tool class key
  id = $1 SUBSEP $2 SUBSEP $5
  if ($9 == "read" || $9 == "search") classed[id]++
  if ($8 == "Edit" || $8 == "Write") edits[id]++
  next
}
{
  # turns.tsv: slug session kind role turn ts ncalls ctx input cache_write cache_read output weight dep
  sid = $1 SUBSEP $2
  if (sid != cur) {
    pendinggroup[1] = pendinggroup[2] = ""
    cur = sid; classrun = editrun = 0
  }
  resolve($7)
  id = $1 SUBSEP $2 SUBSEP $5
  classrun = ($7 == 1 && classed[id] == 1) ? classrun + 1 : 0
  editrun = ($7 >= 1 && edits[id] == $7) ? editrun + 1 : 0
  if ($8 >= line && classrun >= 3) pendinggroup[1] = "lookup" SUBSEP ((($1 SUBSEP $2 SUBSEP $5 SUBSEP "lookup") in nudged) ? "nudged" : "silent")
  if ($8 >= line && editrun >= 2) pendinggroup[2] = "edit" SUBSEP ((($1 SUBSEP $2 SUBSEP $5 SUBSEP "edit") in nudged) ? "nudged" : "silent")
}
END {
  print "breach", "nudged", "turns", "next_batched", "next_batched_share"
  for (breach = 1; breach <= 2; breach++) {
    name = (breach == 1) ? "lookup" : "edit"
    for (g = 1; g <= 2; g++) {
      group = (g == 1) ? "nudged" : "silent"
      key = name SUBSEP group
      share = turns[key] > 0 ? sprintf("%.3f", batched[key] / turns[key]) : ""
      print name, group, turns[key] + 0, batched[key] + 0, share
    }
  }
}' "$dir/events.tsv" "$dir/calls.tsv" "$dir/turns.tsv"
