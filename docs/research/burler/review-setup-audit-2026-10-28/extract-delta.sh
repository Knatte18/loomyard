#!/usr/bin/env bash
# Usage: extract-delta.sh <runs-dir>
# For every finding in round 2 or later of a discussion or plan review, prints whether the text it points at
# was touched by the previous round's fixer, as a token match against that fixer report:
# run perch round id severity class touched
# touched is yes, no, or unkeyed (the finding names no decision `D<n>` and no plan card).
# A discussion finding is keyed by the `D<n>` tokens of its location and summary;
# a plan finding by the card number in its location file name, matched against the integers
# on the fixer report's lines that mention a card.
set -euo pipefail
runs=$1
printf 'run\tperch\tround\tid\tseverity\tclass\ttouched\n'
for f in "$runs"/*/_lyx/reviews/{discussion,plan}/round-*-review.md; do
  [ -e "$f" ] || continue
  round=$(basename "$f" | sed 's/round-\([0-9]*\)-review.md/\1/')
  [ "$round" -ge 2 ] || continue
  dir=$(dirname "$f"); perch=$(basename "$dir")
  run=${f#"$runs"/}; run=${run%%/*}
  prev=$dir/round-$((round - 1))-fixer-report.md
  [ -e "$prev" ] || continue
  awk -v run="$run" -v perch="$perch" -v round="$round" -v prev="$prev" '
    BEGIN {
      while ((getline line < prev) > 0) {
        t = line
        while (match(t, /D[0-9]+/)) { dset[substr(t, RSTART, RLENGTH)] = 1; t = substr(t, RSTART + RLENGTH) }
        if (tolower(line) ~ /card/) {
          t = line
          while (match(t, /[0-9]+/)) { cset[substr(t, RSTART, RLENGTH) + 0] = 1; t = substr(t, RSTART + RLENGTH) }
        }
      }
    }
    /^---$/ { fm++; if (fm == 2) { flush(); exit } next }
    fm != 1 { next }
    /^  - id:/ { flush(); id = $3; sev = cls = loc = sum = ""; next }
    /^    severity:/ { sev = $2 }
    /^    class:/ { cls = $2 }
    /^    location:/ { loc = $0 }
    /^    summary:/ { sum = $0 }
    function flush(   txt, t, keyed, hit, num) {
      if (id == "") return
      txt = loc " " sum; keyed = 0; hit = 0
      if (perch == "discussion") {
        t = txt
        while (match(t, /D[0-9]+/)) { keyed = 1; if (substr(t, RSTART, RLENGTH) in dset) hit = 1; t = substr(t, RSTART + RLENGTH) }
      } else if (match(loc, /plan\/[0-9][0-9]-/)) {
        keyed = 1; num = substr(loc, RSTART + 5, 2) + 0; if (num in cset) hit = 1
      }
      printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", run, perch, round, id, sev, cls, (!keyed ? "unkeyed" : (hit ? "yes" : "no"))
      id = ""
    }
  ' "$f"
done
