#!/usr/bin/env bash
# Usage: extract-twill.sh <runs-dir>
# Prints one TSV row per lens item the twill writers judged, from the writer's own disposition list:
# run writer lens lenses_on_item severity outcome
# writer is twill-discussion (the "lens review" turn of discussion/support-log.md)
# or twill-plan (the bullets of plan/lens-review.md).
# A bullet counts when its first word is a lens name.
# outcome is fixed, rejected (also disputed) or other, from the first "— <word>" or ": <word>" in the bullet.
# lenses_on_item counts the lenses a bullet names (a merged item names several), so 1 means a lens found it alone.
set -euo pipefail
runs=$1
printf 'run\twriter\tlens\tlenses_on_item\tseverity\toutcome\n'
parse='
  function lensname(w) { return w ~ /^(board-fit|bound-breaker|existing-paths|goal-scope|interactions|leanness|walkthrough|coverage|row-validity|forced-edits|guard-reach|way-forward|affected-set)$/ }
  !/^- / { next }
  {
    line = substr($0, 3); split(line, first, /[ ,]/); if (!lensname(first[1])) next
    colon = index(line, ":"); if (colon == 0) next
    head = substr(line, 1, colon - 1); rest = substr(line, colon + 1)
    sub(/ \(.*$/, "", head)
    sev = "-"; if (line ~ /BLOCKING/) sev = "BLOCKING"; else if (line ~ /MEDIUM/) sev = "MEDIUM"; else if (line ~ /LOW/) sev = "LOW"
    out = "other"
    if (match(line, /(— |: )(envelope )?(fixed|rejected|disputed)/)) { w = substr(line, RSTART, RLENGTH); out = (w ~ /fixed/) ? "fixed" : "rejected" }
    n = split(head, parts, " \\+ ")
    for (i = 1; i <= n; i++) {
      split(parts[i], nm, /[ ,]/); if (!lensname(nm[1])) continue
      printf "%s\t%s\t%s\t%d\t%s\t%s\n", run, writer, nm[1], n, sev, out
    }
  }'
for d in "$runs"/*/; do
  run=$(basename "$d")
  sl=$d/_lyx/discussion/support-log.md
  if [ -e "$sl" ] && grep -q 'lens review' "$sl"; then
    awk -v run="$run" -v writer=twill-discussion '/^### / { on = ($0 ~ /lens review/); next } on' "$sl" | awk -v run="$run" -v writer=twill-discussion "$parse"
  fi
  lr=$d/_lyx/plan/lens-review.md
  if [ -e "$lr" ]; then
    awk -v run="$run" -v writer=twill-plan "$parse" "$lr"
  fi
done
