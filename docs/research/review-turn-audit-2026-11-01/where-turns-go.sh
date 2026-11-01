#!/usr/bin/env bash
# Usage: where-turns-go.sh <dir-with-turns-and-calls> > where.tsv
# Where the turns of each role go, from turns.tsv and calls.tsv (the turn audit's extract-turns.sh):
# role group sessions turns calls call_share weight_share time_share
# One row per role and activity group, plus an "all" row per role holding the role's totals.
# A call's group comes from its class:
#   read    read (the Read tool, shell sed -n, cat, head, tail)
#   search  search (shell grep, rg, find, ls, awk)
#   lookup  git-read, lym-read, quarry, skill, and a compound shell command that only reads
#   write   edit, edit-script, scratch-write, other-write
#   check   gate, test-lint, validate
#   fork    agent, message: spawning forks, then waiting for their results
#   other   anything else (ToolSearch, WebFetch, a placeholder call)
# "other-bash" is the extractor's catch-all; a command of it that names no write verb (rm, mv, cp, mkdir, touch, chmod)
# is a compound read (a for loop, go list) and counts under lookup.
# A turn with no call is group "text": the turn ends a stretch of work or answers in prose.
# A no-call turn that follows a turn which spawned forks, before the next write or check call, is group "fork-wait":
# the session wakes on a fork's result, or on the last one, and the turn pays the full context to do so.
# turns counts the turns that hold at least one call of the group; a turn with calls of two groups counts under both.
# call_share is the group's calls over the role's calls.
# weight_share and time_share split each turn evenly among its calls (a no-call turn is all "text") and divide by the role's totals.
# weight is the tokencount weight of the extractor (input + 1.25 x cache write + 0.1 x cache read + 5 x output).
# time is the seconds from a turn's first message to the next turn's, so it covers the turn's generation and its tools' run time;
# a session's last turn takes none.
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' '
function epoch(ts,   y, m, d, era, yoe, doy, doe) {
  # 2026-11-01T07:08:14.697Z -> seconds since 1970-01-01 UTC, milliseconds kept
  y = substr(ts, 1, 4) + 0; m = substr(ts, 6, 2) + 0; d = substr(ts, 9, 2) + 0
  if (m <= 2) y--
  era = int(y / 400); yoe = y - era * 400
  doy = int((153 * (m + (m > 2 ? -3 : 9)) + 2) / 5) + d - 1
  doe = yoe * 365 + int(yoe / 4) - int(yoe / 100) + doy
  return (era * 146097 + doe - 719468) * 86400 + substr(ts, 12, 2) * 3600 + substr(ts, 15, 2) * 60 + substr(ts, 18, 6)
}
function group(cls, key) {
  if (cls == "read") return "read"
  if (cls == "search") return "search"
  if (cls ~ /^(git-read|lym-read|quarry|skill)$/) return "lookup"
  if (cls ~ /^(edit|edit-script|scratch-write|other-write)$/) return "write"
  if (cls ~ /^(gate|test-lint|validate)$/) return "check"
  if (cls ~ /^(agent|message)$/) return "fork"
  if (cls == "other-bash") return (key ~ /(^|[ ;&|])(rm|mv|cp|mkdir|touch|chmod)( |$)/) ? "write" : "lookup"
  return "other"
}
function account(r, key, n, w, gap, waiting,   j, g) {
  tw[r] += w; tt[r] += gap; tturns[r]++; tcalls[r] += n
  if (n == 0) {
    g = waiting ? "fork-wait" : "text"
    groups[g] = 1
    gw[r, g] += w; gt[r, g] += gap; gturns[r, g]++
    return
  }
  delete seen
  for (j = 1; j <= n; j++) {
    g = cg[key, j]
    groups[g] = 1
    gc[r, g]++; gw[r, g] += w / n; gt[r, g] += gap / n
    if (!(g in seen)) { seen[g] = 1; gturns[r, g]++ }
  }
}
FNR == 1 { next }
FILENAME == ARGV[1] {   # calls.tsv: slug session kind role turn call ts tool class key
  cg[$1 SUBSEP $2 SUBSEP $5, $6] = group($9, $10)
  next
}
{                       # turns.tsv: slug session kind role turn ts ncalls ctx input cache_write cache_read output weight dep
  id = $1 SUBSEP $2
  now = epoch($6)
  # the gap to this turn belongs to the previous turn of the same session
  if (id == cur) account(prev_role, prev_key, prev_n, prev_w, now - prev_ts, prev_wait)
  else if (cur != "") account(prev_role, prev_key, prev_n, prev_w, 0, prev_wait)
  if (id != cur) { nsess[$4]++; spawned = 0 }
  roles[$4] = 1
  wait = 0
  if ($7 == 0) wait = spawned
  else for (j = 1; j <= $7; j++) {
    g = cg[id SUBSEP $5, j]
    if (g == "fork") spawned = 1
    else if (g == "write" || g == "check") spawned = 0
  }
  cur = id; prev_wait = wait; prev_ts = now; prev_role = $4; prev_n = $7; prev_w = $13; prev_key = id SUBSEP $5
}
END {
  if (cur != "") account(prev_role, prev_key, prev_n, prev_w, 0, prev_wait)
  print "role", "group", "sessions", "turns", "calls", "call_share", "weight_share", "time_share"
  for (r in roles) {
    printf "%s\tall\t%d\t%d\t%d\t1.000\t1.000\t1.000\n", r, nsess[r], tturns[r], tcalls[r]
    for (g in groups) {
      if (gturns[r, g] == 0) continue
      printf "%s\t%s\t%d\t%d\t%d\t%.3f\t%.3f\t%.3f\n", r, g, nsess[r], gturns[r, g], gc[r, g], (tcalls[r] > 0 ? gc[r, g] / tcalls[r] : 0), gw[r, g] / tw[r], (tt[r] > 0 ? gt[r, g] / tt[r] : 0)
    }
  }
}' "$dir/calls.tsv" "$dir/turns.tsv" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k1,1 -k2,2; }
