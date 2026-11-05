#!/usr/bin/env bash
# Usage: darn-writers.sh <dir-with-turns-and-calls> > darn-writers.tsv
# One row per darn writer main session (role darn), then a TOTAL row, from turns.tsv and calls.tsv (extract-turns.sh of the turn audit):
# slug session turns calls calls_per_turn edit_tool_calls shell_edit_calls awk_mv_edit_calls check_turns lookup_turns
# edit_tool_calls are Edit and Write calls; shell_edit_calls are Bash calls the turn audit classes edit-script (sed -i, perl -i, tee, a heredoc or a redirect into a file);
# awk_mv_edit_calls are the shell edit calls that write through awk and a temporary file moved into place, the route the shell-edit guard leaves open.
# check_turns are turns holding a gate, test, lint or validate call, the darn writer's check loop; lookup_turns are single-call read and search turns.
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' '
function row(sid,    p) {
  split(sid, p, SUBSEP)
  print p[1], p[2], turns[sid], calls[sid], sprintf("%.2f", calls[sid] / turns[sid]), toolcalls[sid] + 0, shellcalls[sid] + 0, awkmv[sid] + 0, checks[sid] + 0, lookups[sid] + 0
}
FNR == 1 { next }
FILENAME == ARGV[1] {
  # calls.tsv: slug session kind role turn call ts tool class key
  if ($3 != "main" || $4 != "darn") next
  sid = $1 SUBSEP $2
  id = sid SUBSEP $5
  nclass[id]++
  cls[id, nclass[id]] = $9
  if ($9 == "edit") toolcalls[sid]++
  if ($9 == "edit-script") {
    shellcalls[sid]++
    if ($10 ~ /awk .*>.*(&&|;) *mv /) awkmv[sid]++
  }
  next
}
{
  # turns.tsv: slug session kind role turn ts ncalls ctx input cache_write cache_read output weight dep
  if ($3 != "main" || $4 != "darn") next
  sid = $1 SUBSEP $2
  if (!(sid in turns)) order[++count] = sid
  turns[sid]++; calls[sid] += $7
  id = sid SUBSEP $5
  hascheck = 0
  for (j = 1; j <= nclass[id]; j++) if (cls[id, j] ~ /^(gate|test-lint|validate)$/) hascheck = 1
  checks[sid] += hascheck
  if ($7 == 1 && (cls[id, 1] == "read" || cls[id, 1] == "search")) lookups[sid]++
}
END {
  print "slug", "session", "turns", "calls", "calls_per_turn", "edit_tool_calls", "shell_edit_calls", "awk_mv_edit_calls", "check_turns", "lookup_turns"
  for (i = 1; i <= count; i++) {
    s = order[i]; row(s)
    tt += turns[s]; tc += calls[s]; tw += toolcalls[s]; ts += shellcalls[s]; ta += awkmv[s]; tk += checks[s]; tl += lookups[s]
  }
  print "TOTAL", count " sessions", tt, tc, sprintf("%.2f", tc / tt), tw, ts, ta, tk, tl
}' "$dir/calls.tsv" "$dir/turns.tsv"
