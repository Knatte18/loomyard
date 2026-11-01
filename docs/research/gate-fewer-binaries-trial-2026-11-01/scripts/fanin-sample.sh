#!/usr/bin/env bash
# fanin-sample.sh <packages file>: prints "<fan-in>\t<package>" over the test binaries of the listed packages
# (module-relative directories, one per line) for tiers 0 to 2 together, sorted descending.
# Fan-in counts a package once per (sample package, tier) test binary that imports it.
set -euo pipefail
list=${1:?file of module-relative package directories}
module=$(go list -m)
for tier in 0 1 2; do
  go list -tags "tier${tier}" -test -f '{{.ImportPath}}|{{join .Deps " "}}' ./... |
  awk -F'|' -v mod="$module" -v list="$list" '
    BEGIN { while ((getline line < list) > 0) want[mod "/" line ".test"] = 1 }
    ($1 in want) {
      n = split($2, deps, " ")
      delete seen
      for (i = 1; i <= n; i++) {
        d = deps[i]; sub(/ \[.*$/, "", d)
        if (index(d, mod) == 1 && !(d in seen)) { seen[d] = 1; print d }
      }
    }'
done | sort | uniq -c | awk '{print $1 "\t" $2}' | sort -t$'\t' -k1,1nr -k2,2
