#!/usr/bin/env bash
# Usage: merge-kinds.sh <dir-with-turns-and-calls> > merge-kinds.tsv
# The calls of the turns a merge would remove, by role and command: role class command turns weight
# A removable turn is a turn of a strict run (session-metrics.sh) other than the run's first.
# The command is the tool name, or for Bash the first word of the first statement after any leading cd or
# variable assignment, with the next word added for lym, git and go.
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' '
function mergeable(c) { return c ~ /^(read|search|git-read|lym-read|quarry|skill)$/ }
function head(tool, key,    s, a) {
  if (tool != "Bash") return tool
  s = key
  while (match(s, /^(cd [^;&|]*(;|&&)|sleep [0-9]+ *(;|&&)|[A-Za-z_][A-Za-z0-9_]*=("[^"]*"|\$\([^)]*\)|[^ ;]*) *;?|for [^;]*;|do|then|if [^;]*;) */)) s = substr(s, RLENGTH + 1)
  split(s, a, " ")
  if (a[1] == "lym" || a[1] == "git" || a[1] == "go") return a[1] " " a[2]
  return a[1]
}
function emit_run(   i) {
  if (rl >= 2) for (i = 2; i <= rl; i++) { n[rrole SUBSEP rc[i] SUBSEP rh[i]]++; w[rrole SUBSEP rc[i] SUBSEP rh[i]] += rwt[i] }
  rl = 0
}
FNR == 1 { next }
FILENAME == ARGV[1] {
  if ($6 == 1) { c1[$1 SUBSEP $2, $5] = $9; h1[$1 SUBSEP $2, $5] = head($8, $10) }
  next
}
{
  id = $1 SUBSEP $2
  if (id != cur) { emit_run(); cur = id }
  t = $5; k = $7; dep = $14
  if (k == 1 && mergeable(c1[id, t])) {
    if (dep == 1) emit_run()
    rl++; rrole = $4; rc[rl] = c1[id, t]; rh[rl] = h1[id, t]; rwt[rl] = $13
  } else emit_run()
}
END {
  emit_run()
  print "#"
  for (key in n) { split(key, p, SUBSEP); print p[1], p[2], p[3], n[key], int(w[key]) }
}
' "$dir/calls.tsv" "$dir/turns.tsv" | { read -r _; printf "role\tclass\tcommand\tremovable_turns\tremovable_weight\n"; sort -t$'\t' -k1,1 -k4,4nr; }
