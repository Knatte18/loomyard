#!/usr/bin/env bash
# Usage: pure-logic-candidates.sh <out dir> <results dir> > purelogic.tsv
# Run analyze-coverage.sh and git-census.sh first (gitprocs.tsv in <results dir>), and a tier 0 run of the package with
#   -coverpkg=./internal/fabricengine -coverprofile=<out dir>/tier0.out
# Prints one line per tier 1 test: test, cost seconds, git processes, rare blocks only tier 1 covers, rare blocks tier 0 covers too.
# A rare block is one that at most 5 % of the tier 1 tests cover; the rest is the code that builds fixtures.
# A tier 0 test starts no git, so a rare block it covers runs without git.
# A test whose rare blocks are mostly tier 0 blocks exercises logic a tier 0 test could reach, and starts git for its fixture.
# Needs bash and awk; reads only.
set -euo pipefail
export LC_ALL=C

out=$1
results=$2
awk -F'\t' -v tier0="$out/tier0.out" -v gitfile="$results/gitprocs.tsv" -v costfile="$out/cost.tsv" '
  BEGIN {
    while ((getline line < tier0) > 0) { n = split(line, f, " "); if (n == 3 && f[3] > 0) t0[f[1]] = 1 }
    while ((getline line < gitfile) > 0) { split(line, f, "\t"); gits[f[1]] = f[2] }
    while ((getline line < costfile) > 0) { split(line, f, "\t"); cost[f[1]] = f[2]; tests[++nt] = f[1] }
  }
  { cnt[$2]++; nb[$1]++; blk[$1, nb[$1]] = $2 }
  END {
    for (i = 1; i <= nt; i++) {
      t = tests[i]; beyond = 0; needGit = 0
      for (j = 1; j <= nb[t]; j++) {
        b = blk[t, j]
        if (cnt[b] > 0.05 * nt) continue
        beyond++
        if (!(b in t0)) needGit++
      }
      printf "%s\t%s\t%d\t%d\t%d\n", t, cost[t], gits[t], needGit, beyond - needGit
    }
  }' "$out/covered.tsv"
