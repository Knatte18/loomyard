#!/usr/bin/env bash
# place-status.sh <pair> <ref-slug> <records-commit>: puts the pair's run onto the fix row the reference's
# records commit ("<P>-Burler-Fix -> running" of round N) stands on.
# It takes the reference's status.json from that commit, keeps the pair's own Preflight and Loom-Preflight entries,
# rewrites every output path from the reference's worktree to the pair's and the product slug to the pair's,
# and writes it over the pair's status.json; the original status is kept beside the run's evidence.
H=/home/knatte/Code/loomyard-LYXHUB
S=$H/loomyard/.scratch/burler-twill-fixer-bench/setup
pair=$1 ref=$2 rec=$3
st=$H/$pair/_lyx/shed/$pair/status.json
[ -f "$st" ] || { echo "no status at $st"; exit 1; }
mkdir -p "$S/status"
cp "$st" "$S/status/$pair.before.json"
git -C "$H/loomyard-weft" show "$rec:_lyx/shed/$ref/status.json" > "$S/status/$pair.ref.json" || exit 1
jq --slurpfile own "$S/status/$pair.before.json" --arg from "/$ref/" --arg to "/$pair/" --arg pair "$pair" '
  .history = ($own[0].history[0:2] + [ .history[] | select(.producer != "Preflight" and .producer != "Loom-Preflight") | .output |= gsub($from; $to) ])
  | .product.slug = $pair
  | .pause_requested = false
' "$S/status/$pair.ref.json" > "$S/status/$pair.placed.json" || exit 1
cp "$S/status/$pair.placed.json" "$st"
jq -c '{current_producer, state, last: .activity.last, n: (.history|length), tail: [.history[-3:][] | .producer + ":" + .outcome]}' "$st"
