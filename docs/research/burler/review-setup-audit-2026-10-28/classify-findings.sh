#!/usr/bin/env bash
# Usage: classify-findings.sh <runs-dir>
# Prints one TSV row per gating (BLOCKING or MEDIUM) finding of every discussion and plan round review:
# run perch round id theme
# theme is the first rule below whose pattern matches the finding's one-line summary and location, else other.
# The rules are keyword heuristics, applied in order, so a finding takes one theme;
# a summary written as a YAML block scalar has no first line to match and lands in other.
set -euo pipefail
runs=$1
printf 'run\tperch\tround\tid\ttheme\n'
for f in "$runs"/*/_lyx/reviews/{discussion,plan}/round-*-review.md; do
  [ -e "$f" ] || continue
  run=${f#"$runs"/}; run=${run%%/*}
  perch=$(basename "$(dirname "$f")")
  round=$(basename "$f" | sed 's/round-\([0-9]*\)-review.md/\1/')
  awk -v run="$run" -v perch="$perch" -v round="$round" '
    /^---$/ { fm++; if (fm == 2) { flush(); exit } next }
    fm != 1 { next }
    /^  - id:/ { flush(); id = $3; sev = loc = sum = ""; next }
    /^    severity:/ { sev = $2 }
    /^    location:/ { loc = $0 }
    /^    summary:/ { sum = $0 }
    function flush(   t, theme) {
      if (id == "" || (sev != "BLOCKING" && sev != "MEDIUM")) { id = ""; return }
      t = tolower(sum " " loc)
      theme = "other"
      if (t ~ /pattern-|pattern\.md|pattern entry|line cap/) theme = "pattern-rule"
      else if (t ~ /tier tag|tier[0-9]|nil-seam|nil seam|fixture|testing|every env|everyfield|reflective|test-speed|test-isolation/) theme = "test-seam-or-tier"
      else if (t ~ /doc\.go|docs\/overview|readme|skill\.md| docs /) theme = "docs-sync"
      else if (t ~ /bound|unbounded|lets through|what it can skip|bypass/) theme = "unbounded-edge-or-guard"
      else if (t ~ /way forward|names no next|dead end|loops back|next step/) theme = "refusal-way-forward"
      else if (t ~ /acceptance|no row|unreached|untested|no test|no card|not covered|unpinned/) theme = "coverage-gap"
      else if (t ~ /contradict|disagree|inconsisten|conflict|against d[0-9]|renamed|still names|stale|restat|duplicate/) theme = "cross-section-inconsistency"
      else if (t ~ /does not exist|no such|the code|reads |resolves|calls |function|returns |wrong|false/) theme = "code-claim-wrong"
      else if (t ~ /order|before card|cannot pass|own gate|depends on card|card [0-9]+ .* card [0-9]+/) theme = "card-order-or-dependency"
      printf "%s\t%s\t%s\t%s\t%s\n", run, perch, round, id, theme
      id = ""
    }
  ' "$f"
done
