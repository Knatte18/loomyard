#!/usr/bin/env bash
# fanin-union.sh [packages file]: prints "<fan-in>\t<package>" where fan-in is the number of test binaries that import
# the package when every tier's test files compile into one binary per package (tiers 0 to 3 unioned, the
# one-binary-per-package layout). With a packages file (module-relative directories, one per line) only those
# packages' binaries count; without it all do. A first line "#binaries\t<n>" gives the binary count.
set -euo pipefail
list=${1:-}
module=$(go list -m)
for tier in 0 1 2 3; do
  go list -tags "tier${tier}" -test -f '{{.ImportPath}}|{{join .Deps " "}}' ./... |
  awk -F'|' -v mod="$module" -v list="$list" '
    BEGIN { if (list != "") while ((getline line < list) > 0) want[mod "/" line ".test"] = 1 }
    $1 ~ /\.test$/ && (list == "" || ($1 in want)) {
      n = split($2, deps, " ")
      print $1 "\t" $1
      for (i = 1; i <= n; i++) {
        d = deps[i]; sub(/ \[.*$/, "", d)
        if (index(d, mod) == 1) print $1 "\t" d
      }
    }'
done | sort -u | awk -F'\t' '
  $1 == $2 { binaries++; next }
  { fan[$2]++ }
  END { printf "#binaries\t%d\n", binaries; for (d in fan) printf "%d\t%s\n", fan[d], d }' |
  { read -r head; echo "$head"; sort -t$'\t' -k1,1nr -k2,2; }
