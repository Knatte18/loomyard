#!/usr/bin/env bash
# Usage: tables.sh [dir]
# Prints the report's group tables as markdown from <dir>/cost-table.tsv (default: this script's directory).
# The sample is the darn runs with a landing commit, less EXCLUDE (default `comment-cleanup`, a one-off run whose
# comment-lead seat cost $335 and which is not a normal darn).
# The per-1000-line table divides the cost of the runs of 1 to MAXLINES (default 10000) changed lines by their lines;
# larger runs are research darns that commit data tables, and runs of no changed line have nothing to divide by.
# Sections, each headed by a line starting `## `:
#   darn means by group, darn per 1000 changed lines by group, writer against Code-Review perch by group,
#   tapestry runs per run, tapestry per 1000 changed lines.
# Needs bash and awk; prints to standard output only.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
dir=${1:-$here}
exclude=${EXCLUDE:-comment-cleanup}
maxlines=${MAXLINES:-10000}

awk -F'\t' -v exclude="$exclude" -v maxlines="$maxlines" '
function group(profile, stored) {
  if (profile == "-") return "before profiles (no usage record)"
  return profile (stored == "true" ? ", stored record" : "")
}
function add(g, i) { sum[g, i] += $i }
function row(g,   n, lines) {
  n = cnt[g]
  printf "| %s | %d | %.0f | %.0f | %.1f |", g, n, sum[g, 6] / n, sum[g, 7] / n, sum[g, 20] / n
  # discussion, writer, writer forks, reviewers, bouncer, fixers, gate fixer, landing + other, total
  printf " $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f |\n", sum[g, 8] / n, sum[g, 11] / n, sum[g, 12] / n, sum[g, 13] / n, sum[g, 14] / n, sum[g, 15] / n, sum[g, 16] / n, (sum[g, 17] + sum[g, 18]) / n, sum[g, 19] / n
}
function per1000(g,   l) {
  l = sum2[g, 6] / 1000
  if (l == 0) { printf "| %s | 0 | | | | | | | | | |\n", g; return }
  printf "| %s | %d | %.1f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | %.1f |\n", g, cnt2[g], l, sum2[g, 11] / l + sum2[g, 12] / l, sum2[g, 13] / l, sum2[g, 14] / l, sum2[g, 15] / l, sum2[g, 16] / l, (sum2[g, 17] + sum2[g, 18] + sum2[g, 8]) / l, sum2[g, 19] / l, sum2[g, 7] / l
}
function perch(g,   w, p, t) {
  w = sum[g, 11] + sum[g, 12]; p = sum[g, 13] + sum[g, 14] + sum[g, 15]; t = sum[g, 19]
  printf "| %s | %d | $%.2f | %.0f%% | $%.2f | %.0f%% | %s | $%.2f |\n", g, cnt[g], w / cnt[g], 100 * w / t, p / cnt[g], 100 * p / t, (p > 0 ? sprintf("%.2f", w / p) : "no review"), t / cnt[g]
}
NR == 1 { next }
$2 == "darn" && $6 != "" && $1 != exclude {
  g = group($4, $5)
  for (i = 6; i <= 21; i++) { add(g, i); add("all landed darns", i) }
  cnt[g]++; cnt["all landed darns"]++
  if (!(g in seen)) { seen[g] = 1; order[++ng] = g }
  if ($6 > 0 && $6 <= maxlines) {
    for (i = 6; i <= 21; i++) { sum2[g, i] += $i; sum2["all landed darns", i] += $i }
    cnt2[g]++; cnt2["all landed darns"]++
  }
}
$2 == "tapestry" && $6 != "" {
  nt++; tp[nt] = $0
  for (i = 6; i <= 21; i++) add("tapestry", i)
  cnt["tapestry"]++
}
END {
  # order: the recorded profiles first, alphabetically; the unrecorded group last
  for (i = 1; i <= ng; i++) for (j = i + 1; j <= ng; j++) {
    a = order[i]; b = order[j]
    ka = (a ~ /^before/) ? "z" a : a; kb = (b ~ /^before/) ? "z" b : b
    if (ka > kb) { order[i] = b; order[j] = a }
  }
  print "## darn means by group"
  print "| group | runs | changed lines | wall min | review rounds | Discussion | writer | writer forks | reviewers | judge and seed | fixers | gate fixer | landing and other | total |"
  print "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
  for (i = 1; i <= ng; i++) row(order[i])
  row("all landed darns")
  print ""
  print "## darn per 1000 changed lines by group"
  print "| group | runs | thousand lines | writer and forks | reviewers | judge and seed | fixers | gate fixer | other | total | wall min |"
  print "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
  for (i = 1; i <= ng; i++) per1000(order[i])
  per1000("all landed darns")
  print ""
  print "## writer against Code-Review perch by group"
  print "| group | runs | writer and forks per run | share of total | perch per run | share of total | writer over perch | total per run |"
  print "| --- | --- | --- | --- | --- | --- | --- | --- |"
  for (i = 1; i <= ng; i++) perch(order[i])
  perch("all landed darns")
}
' "$dir/cost-table.tsv"
