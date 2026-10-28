#!/usr/bin/env bash
# Usage: extract-rounds.sh <runs-dir>
# Prints one TSV row per review round of every run under <runs-dir>, from the round's usage record,
# its review front matter and its bouncer ledger:
# run perch round fan review_model review_effort review_wall_s review_fresh review_cache forks fork_fresh
#   fix_wall_s fix_fresh review_verdict bouncer_verdict findings gating new_ledger_keys
# A fresh-token count is uncached input plus output; cache reads are listed apart.
set -euo pipefail
runs=$1
printf 'run\tperch\tround\tfan\treview_model\treview_effort\treview_wall_s\treview_fresh\treview_cache\tforks\tfork_fresh\tfix_wall_s\tfix_fresh\treview_verdict\tbouncer_verdict\tfindings\tgating\tnew_ledger_keys\n'
for u in "$runs"/*/_lyx/reviews/*/round-*-usage.yaml; do
  run=${u#"$runs"/}; run=${run%%/*}
  dir=$(dirname "$u"); perch=$(basename "$dir")
  round=$(basename "$u" | sed 's/round-\([0-9]*\)-usage.yaml/\1/')
  usage=$(awk '
    /^fan:/ { fan = $2; gsub(/"/, "", fan) }
    /^[a-z_]+:/ { sec = $1; sub(":", "", sec) }
    sec == "review" && /^    model:/ { rm = $2 }
    sec == "review" && /^    effort:/ { re = $2 }
    sec == "review" && /^    wall_seconds:/ { rw = $2 }
    sec == "review" && /^    fresh_tokens:/ { rf = $2 }
    sec == "review" && /^    cache_read_tokens:/ { rc = $2 }
    sec == "review" && /^    forks:/ { fk = $2 }
    sec == "review" && /^    fork_fresh_tokens:/ { ff = $2 }
    sec == "fix" && /^    wall_seconds:/ { fw = $2 }
    sec == "fix" && /^    fresh_tokens:/ { fx = $2 }
    END { printf "%s\t%s\t%s\t%d\t%d\t%s\t%d\t%d\t%d", fan, rm, re, rw, rf, rc, fk, ff, fw; printf "\t%d", fx }
  ' "$u")
  rv=$dir/round-$round-review.md
  verdict=$(awk '/^verdict:/ {print $2; exit}' "$rv" 2>/dev/null || true)
  bv=$(awk '/^verdict:/ {print $2; exit}' "$dir/round-$round-bouncer-verdict.md" 2>/dev/null || true)
  n=$(grep -c '^  - id:' "$rv" 2>/dev/null || true)
  g=$(awk '/^    severity:/ && ($2=="BLOCKING"||$2=="HIGH"||$2=="MEDIUM") {c++} END{print c+0}' "$rv" 2>/dev/null || echo 0)
  nk=$(awk -v r="$round" '/^  - key:/ {k=1} /rounds:/ { s=$0; sub(/.*\[/, "", s); sub(/,.*|\].*/, "", s); if (s == r) c++ } END {print c+0}' "$dir/round-$round-bouncer-ledger.md" 2>/dev/null || echo 0)
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$run" "$perch" "$round" "$usage" "$verdict" "$bv" "${n:-0}" "$g" "$nk"
done
