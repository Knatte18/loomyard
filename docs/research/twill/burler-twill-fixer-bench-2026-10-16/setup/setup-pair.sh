#!/usr/bin/env bash
# setup-pair.sh <pair> <ref-slug> <code-base> <records-commit> <type-label> <title>:
# makes one bench pair standing on the reference's round-N fix row, as wave 1 was made by hand:
# a twb-* note carrying the reference's board body, lyx fabric add from the prime, the code branch reset to the base,
# the frozen discussion, plan and review records checked out from the records commit, the pair's claude and loom config,
# seed, Preflight and Loom-Preflight, then the status placed onto the fix row (place-status.sh).
H=/home/knatte/Code/loomyard-LYXHUB
B=$H/loomyard/.scratch/burler-twill-fixer-bench
S=$B/setup
pair=$1 ref=$2 base=$3 rec=$4 label=$5 title=$6
log=$S/setup-$pair.log
set -o pipefail
step() { echo "$(date +%T) $*" >> "$log"; "$@" >> "$log" 2>&1 || { echo "FAIL: $*" | tee -a "$log"; exit 1; }; echo >> "$log"; }
cd "$H/loomyard" || exit 1
payload="$(jq -n -c --arg s "$pair" --arg t "$title" --arg l "$label" '{slug:$s, kind:"note", labels:[$l], title:$t}')"
step lyx board upsert "$payload" --body-file "$B/keys/$ref/board-body.md"
step lyx fabric add "$pair"
step git -C "$H/$pair" reset -q --hard "$base"
paths=()
for d in _lyx/discussion _lyx/plan _lyx/reviews/discussion _lyx/reviews/plan; do
  git -C "$H/loomyard-weft" cat-file -e "$rec:$d" 2>/dev/null && paths+=("$d")
done
step git -C "$H/$pair-weft" checkout "$rec" -- "${paths[@]}"
cd "$H/$pair" || exit 1
step lyx config claude --set "binary=$S/wrap-btf.sh" --set deny_agent_tool=false
step lyx config loom --set 'plan_fix=opus[medium]' --set 'plan_review=opus[medium]' --set 'discussion_fix=opus[medium]' --set 'discussion_review=opus[medium]' --set skip_alignment=true --set discussion_max_bounces=4 --set discussion_hard_max_bounces=5 --set plan_max_bounces=4 --set plan_hard_max_bounces=5
step lyx fabric commit
step lyx shed seed self --recipe loom --driver go --param parent=main
step lyx loom step
step lyx loom step
step "$S/place-status.sh" "$pair" "$ref" "$rec"
echo "$pair ready: $(git -C "$H/$pair" log -1 --format=%h) $(jq -r .current_producer "$H/$pair/_lyx/shed/$pair/status.json")"
