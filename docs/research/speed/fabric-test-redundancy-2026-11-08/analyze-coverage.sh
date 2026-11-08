#!/usr/bin/env bash
# Usage: analyze-coverage.sh <out dir> <results dir>
# Reads what coverage-runner.sh left in <out dir> and writes to <results dir>:
#   pertest.tsv    test, cost seconds, covered blocks, blocks only this test covers, tests covering its rarest block (itself included)
#   contained.tsv  the tests whose every covered block another test also covers, by cost, slowest first
#   greedy.tsv     contained tests dropped together, slowest first, each one's blocks still covered by a test left in
#   summary.txt    the totals the report quotes
# A test is tier 1 here when its log holds no `--- SKIP` line for the top-level test.
# cost is the larger of the test's own line and the sum of its leaf subtests' lines, as the per-test times report ranks.
# A block is a coverage profile line's file and range, covered when its count is above zero.
# It also leaves cost.tsv and covered.tsv (test, covered block) in <out dir> for pure-logic-candidates.sh.
# Needs bash and awk; reads and writes only the two directories.
set -euo pipefail
export LC_ALL=C

out=$1
results=$2
mkdir -p "$results"
work=$out

# Tier 1 tests and their cost.
for log in "$out"/logs/*.log; do
  name=$(basename "$log" .log)
  if grep -q -- "^--- SKIP: $name " "$log"; then continue; fi
  awk -v name="$name" '
    /^[ ]*--- PASS: / {
      line = $3; sec = $4; gsub(/[()s]/, "", sec); own[line] = sec + 0; order[++n] = line
    }
    END {
      top = (name in own) ? own[name] : 0
      for (i = 1; i <= n; i++) { p = order[i]; sub(/\/[^\/]*$/, "", p); if (p != order[i]) hasChild[p] = 1 }
      leaves = 0
      for (i = 1; i <= n; i++) if (!(order[i] in hasChild) && order[i] != name) leaves += own[order[i]]
      printf "%s\t%.2f\n", name, (leaves > top ? leaves : top)
    }' "$log"
done | sort > "$work/cost.tsv"

# Covered blocks per tier 1 test.
cut -f1 "$work/cost.tsv" | while read -r name; do
  awk -v name="$name" 'NR > 1 && $3 > 0 { print name "\t" $1 }' "$out/profiles/$name.out"
done > "$work/covered.tsv"

awk -F'\t' -v costfile="$work/cost.tsv" -v results="$results" '
  BEGIN { while ((getline line < costfile) > 0) { split(line, f, "\t"); cost[f[1]] = f[2]; tests[++nt] = f[1] } }
  { cnt[$2]++; nb[$1]++; blk[$1, nb[$1]] = $2 }
  END {
    for (b in cnt) total++
    for (i = 1; i <= nt; i++) {
      t = tests[i]; uniq[t] = 0; rarest = 1000000
      for (j = 1; j <= nb[t]; j++) { c = cnt[blk[t, j]]; if (c == 1) uniq[t]++; if (c < rarest) rarest = c }
      printf "%s\t%s\t%d\t%d\t%d\n", t, cost[t], nb[t], uniq[t], rarest > (results "/pertest.tsv")
      if (uniq[t] == 0) { nc++; csec += cost[t]; printf "%s\t%s\t%d\n", t, cost[t], nb[t] > (results "/contained.unsorted") }
    }
    # Greedy: walk the contained tests slowest first, drop one when every block it covers has another test left.
    n = 0
    close(results "/contained.unsorted")
    while ((getline line < (results "/contained.unsorted")) > 0) { split(line, f, "\t"); order[++n] = f[1]; ocost[f[1]] = f[2] }
    close(results "/contained.unsorted")
    # selection sort by cost, slowest first
    for (i = 1; i <= n; i++) { best = i; for (j = i + 1; j <= n; j++) if (ocost[order[j]] + 0 > ocost[order[best]] + 0) best = j; tmp = order[i]; order[i] = order[best]; order[best] = tmp }
    for (i = 1; i <= n; i++) {
      t = order[i]; ok = 1
      for (j = 1; j <= nb[t]; j++) if (cnt[blk[t, j]] < 2) { ok = 0; break }
      printf "%s\t%s\t%d\n", t, ocost[t], nb[t] > (results "/contained.tsv")
      if (ok) {
        for (j = 1; j <= nb[t]; j++) cnt[blk[t, j]]--
        ng++; gsec += ocost[t]; printf "%s\t%s\t%d\n", t, ocost[t], nb[t] > (results "/greedy.tsv")
      }
    }
    totalsec = 0; for (i = 1; i <= nt; i++) totalsec += cost[tests[i]]
    printf "tier 1 top-level tests\t%d\ncost of all, seconds\t%.1f\ncovered blocks in the union\t%d\ncontained tests\t%d\ncost of contained, seconds\t%.1f\ngreedy droppable together\t%d\ncost of greedy set, seconds\t%.1f\n", nt, totalsec, total, nc, csec, ng, gsec > (results "/summary.txt")
  }' "$work/covered.tsv"
rm -f "$results/contained.unsorted"
