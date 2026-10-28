#!/usr/bin/env bash
# Usage: extract-findings.sh <runs-dir>
# Prints one TSV row per finding in every round review of every run under <runs-dir>:
# run perch round id severity class origin
# <runs-dir>/<slug> holds the unpacked archive tag of one landed run.
set -euo pipefail
runs=$1
printf 'run\tperch\tround\tid\tseverity\tclass\torigin\n'
for f in "$runs"/*/_lyx/reviews/*/round-*-review.md; do
  run=${f#"$runs"/}; run=${run%%/*}
  perch=$(basename "$(dirname "$f")")
  round=$(basename "$f" | sed 's/round-\([0-9]*\)-review.md/\1/')
  awk -v run="$run" -v perch="$perch" -v round="$round" '
    /^---$/ { fm++; if (fm == 2) { flush(); exit } next }
    fm != 1 { next }
    /^  - id:/ { flush(); id = $3; sev = cls = org = "" ; next }
    /^    severity:/ { sev = $2 }
    /^    class:/ { cls = $2 }
    /^    origin:/ { org = $2 }
    function flush() { if (id != "") printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", run, perch, round, id, sev, cls, org; id = "" }
    END { flush() }
  ' "$f"
done
