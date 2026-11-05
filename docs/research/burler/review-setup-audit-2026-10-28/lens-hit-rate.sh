#!/usr/bin/env bash
# Usage: lens-hit-rate.sh <runs-dir>
# Prints one TSV row per review lens and perch kind:
# perch lens rounds_ran rounds_with_finding rounds_with_gating rounds_with_blocking
# A lens ran in a round when the round's usage record lists it; a finding is the lens's origin on a kept finding,
# gating is BLOCKING or MEDIUM. A code perch counts every code review round, archived re-reviews included.
set -euo pipefail
runs=$1
printf 'perch\tlens\trounds_ran\trounds_with_finding\trounds_with_gating\trounds_with_blocking\n'
for u in "$runs"/*/_lyx/reviews/*/round-*-usage.yaml; do
  run=${u#"$runs"/}; run=${run%%/*}
  dir=$(dirname "$u"); perch=$(basename "$dir"); perch=${perch%%-*}
  round=$(basename "$u" | sed 's/round-\([0-9]*\)-usage.yaml/\1/')
  awk '/^lenses:/ {on=1; next} /^[a-z_]+:/ {on=0} on && /^    - / {print $2}' "$u" | while read -r lens; do
    f=$dir/round-$round-review.md
    awk -v perch="$perch" -v lens="$lens" '
      /^---$/ { fm++; if (fm == 2) exit; next }
      fm != 1 { next }
      /^  - id:/ { sev = "" }
      /^    severity:/ { sev = $2 }
      /^    origin:/ { if ($2 == "lens:" lens) { any = 1; if (sev == "BLOCKING" || sev == "MEDIUM") gate = 1; if (sev == "BLOCKING") blk = 1 } }
      END { printf "%s\t%s\t1\t%d\t%d\t%d\n", perch, lens, any, gate, blk }
    ' "$f"
  done
done | awk -F'\t' 'NR==FNR{} {k=$1"\t"$2; a[k]+=$3; b[k]+=$4; c[k]+=$5; d[k]+=$6} END{for(k in a) printf "%s\t%d\t%d\t%d\t%d\n",k,a[k],b[k],c[k],d[k]}' | sort
