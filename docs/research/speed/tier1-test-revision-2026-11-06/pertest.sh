#!/usr/bin/env bash
# Usage: pertest.sh <gate-log> > pertest.tsv
# Prints one line per top-level test a gate log holds a `--- PASS` line for: package, test, cost, own.
# own is the test's own seconds; cost adds every subtest that paused (called t.Parallel), folded into its nearest named ancestor, so cost follows the work a parallel test did and exceeds own where subtests ran beside each other.
# A gate log holds the text of `go test -json` with the packages interleaved, so a test is tied to its package by the one `func Test<name>` that declares it; a name declared in two packages is dropped.
# Needs bash, awk and grep; run from the repository root; reads only. The log is the path `lym gate test` prints at its end.
set -euo pipefail
export LC_ALL=C
log=$1

declared=$(mktemp)
trap 'rm -f "$declared"' EXIT
grep -rH --include='*_test.go' -E '^func Test[A-Za-z0-9_]*\(' . |
  awk -F: '{ dir = $1; sub(/^\.\//, "", dir); sub(/\/[^\/]*$/, "", dir); name = $2; sub(/^func /, "", name); sub(/\(.*/, "", name); print dir "\t" name }' |
  sort -u | awk -F'\t' '{ seen[$2]++; pkg[$2] = $1 } END { for (n in seen) if (seen[n] == 1) print pkg[n] "\t" n }' |
  sort -t$'\t' -k2,2 > "$declared"

awk '
function depth(s,   t) { t = s; return gsub("/", "/", t) }
/^=== PAUSE / { paused[$3] = 1; next }
/^[ ]*--- PASS: / { name = $3; seconds = $4; gsub(/[()s]/, "", seconds); own[name] = seconds + 0; next }
END {
    maxdepth = 0
    for (name in own) { d = depth(name); dep[name] = d; if (d > maxdepth) maxdepth = d; cost[name] = own[name] }
    for (d = maxdepth; d >= 1; d--)
        for (name in own)
            if (dep[name] == d && (name in paused)) {
                parent = name
                while (parent != "" && !((parent in own) && parent != name)) { if (!sub(/\/[^\/]*$/, "", parent)) { parent = ""; break } }
                if (parent != "") cost[parent] += cost[name]
            }
    for (name in own) if (dep[name] == 0) printf "%s\t%.2f\t%.2f\n", name, cost[name], own[name]
}' "$log" | sort -t$'\t' -k1,1 |
  join -t$'\t' -1 2 -2 1 "$declared" - |
  awk -F'\t' '{ print $2 "\t" $1 "\t" $3 "\t" $4 }'
