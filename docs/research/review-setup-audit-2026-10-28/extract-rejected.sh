#!/usr/bin/env bash
# Usage: extract-rejected.sh <runs-dir>
# Prints one TSV row per lens item a round review's `## Rejected` section drops:
# run perch round lens outcome
# outcome is folded when the bullet says it was folded, merged or duplicated into another finding, otherwise dropped.
set -euo pipefail
runs=$1
printf 'run\tperch\tround\tlens\toutcome\n'
for f in "$runs"/*/_lyx/reviews/*/round-*-review.md; do
  run=${f#"$runs"/}; run=${run%%/*}
  perch=$(basename "$(dirname "$f")")
  round=$(basename "$f" | sed 's/round-\([0-9]*\)-review.md/\1/')
  awk -v run="$run" -v perch="$perch" -v round="$round" '
    /^## / { on = ($0 ~ /^## Rejected/); next }
    on && /^- / {
      line = substr($0, 3)
      if (!match(line, /^[a-z]+(-[a-z]+)*/)) next
      lens = substr(line, RSTART, RLENGTH)
      if (lens !~ /^(board-fit|bound-breaker|existing-paths|goal-scope|interactions|leanness|walkthrough|code-correctness|code-affected-set|code-tests|code-pattern-fit|affected-set|guard-reach|way-forward)$/) next
      out = (tolower(line) ~ /fold|merge|duplicate|same as|covered by/) ? "folded" : "dropped"
      printf "%s\t%s\t%s\t%s\t%s\n", run, perch, round, lens, out
    }
  ' "$f"
done
