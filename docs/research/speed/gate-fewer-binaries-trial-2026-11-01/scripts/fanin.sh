#!/usr/bin/env bash
# fanin.sh <tier>: prints "<fan-in>\t<package>" for every module package, sorted descending,
# where fan-in is the number of test binaries the tier links that import the package.
# A first line "#binaries\t<n>" gives the tier's test-binary count.
# Reads the package graph only (go list); nothing is built or run.
set -euo pipefail
tier=${1:?tier number}
module=$(go list -m)
go list -tags "tier${tier}" -test -f '{{.ImportPath}}|{{join .Deps " "}}' ./... |
awk -F'|' -v mod="$module" '
  $1 ~ /\.test$/ {
    binaries++
    n = split($2, deps, " ")
    delete seen
    for (i = 1; i <= n; i++) {
      d = deps[i]
      sub(/ \[.*$/, "", d)
      if (index(d, mod) == 1 && !(d in seen)) { seen[d] = 1; fan[d]++ }
    }
  }
  END {
    printf "#binaries\t%d\n", binaries
    for (d in fan) printf "%d\t%s\n", fan[d], d
  }' | { read -r head; echo "$head"; sort -t$'\t' -k1,1nr -k2,2; }
