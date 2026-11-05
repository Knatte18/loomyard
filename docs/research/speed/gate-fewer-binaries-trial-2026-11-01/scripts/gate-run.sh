#!/usr/bin/env bash
# gate-run.sh <label> [target...]: runs tier0, tier1 and tier2 of the gate once each, over the given targets
# (default --all, the whole suite), and appends one TSV row per tier to $RESULTS:
# label, tier, launches (test binaries new to the cache), discovered, passed, failed, skipped, wall_seconds, launch_seconds.
# Each tier runs with LYM_TEST_TIER set to its number, the run-time tier selector trial 2 reads; trial 1 ignores it.
# A tier's gate output replaces $here/last-gate-output-tier<N>.txt.
# The packages each tier launched are listed in launched/<label>-tier<N>.txt beside the results.
set -uo pipefail
label=${1:?label}
shift
targets=("$@")
[ ${#targets[@]} -gt 0 ] || targets=(--all)
here=$(cd "$(dirname "$0")/.." && pwd)
results=${RESULTS:-$here/results.tsv}
runs=/home/knatte/Code/loomyard-LYMHUB/_bolt/.lym/gate/runs
status=0
for tier in 0 1 2; do
  marker=$(mktemp -p .scratch marker.XXXXXX)
  LYM_TEST_TIER=$tier lym gate test --wait --tier "$tier" "${targets[@]}" >"$here/last-gate-output-tier${tier}.txt" 2>&1 || status=1
  record=$(find "$runs" -name "*-tier${tier}-*.json" -newer "$marker" | sort | while read -r candidate; do
    [ "$(jq -r .worktree "$candidate")" = "$PWD" ] && echo "$candidate"
  done | tail -1)
  [ -n "$record" ] || { echo "no tier$tier record after $label" >&2; continue; }
  jq -r --arg label "$label" --argjson tier "$tier" '
    [$label, $tier, (.launches // [] | length), .discovered, .passed, .failed, .skipped,
     (.wall_seconds * 10 | round / 10), ((.launches // []) | map(.launch_seconds) | add // 0 | . * 10 | round / 10)] | @tsv' \
    "$record" >>"$results"
  mkdir -p "$here/launched"
  jq -r '.launches[]?.package' "$record" | sort >"$here/launched/${label}-tier${tier}.txt"
done
exit $status
