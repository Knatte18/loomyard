#!/usr/bin/env bash
# Usage: verify.sh <slug> <tag>
# Prints the run's verify gate spans from its status at the archive tag,
# then per archived verify log the summed tier wall, the unreached functions of its reach note,
# its triage lines, its failing tiers and its mutation report line.
# Needs bash, jq and git; reads only.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
slug=$1 tag=$2
logs=$(ls -d "$ARCHIVE/$slug"/*/verify)
echo "## spans"
git -C "$WEFT" show "$tag:_lym/shed/$slug/status.json" | jq -r '
  def e: sub("\\.[0-9]+";"") | fromdateiso8601;
  .history[] | .producer as $p | (.gate_spans // [])[] | select(.gate=="verify")
  | "\($p) \(.kind) \(.start[0:19]) \(.end[0:19]) \(((.end|e)-(.start|e)))s"'
echo "## logs"
for f in $(ls "$logs"/verify-*.log | sort -t- -k2 -n); do
  # Only lines naming the hub's own gate run records: a verify log also holds the output of tests that run lym gate test.
  wall=$(grep "^gate test: tier.* log $HUB/" "$f" | grep -o ' wall [0-9.]*s' | grep -o '[0-9.]*' | awk '{ s += $1 } END { printf "%.1f", s }')
  unreached=$(grep -c '^gate test: reach: no mapped test reaches' "$f")
  triage=$(grep -c 'triage: ' "$f")
  failed=$(grep "^gate test: tier.* log $HUB/" "$f" | grep -o '^gate test: tier[0-9][^,]* failed [1-9][0-9]*' | tr '\n' ';')
  mutation=$(grep "^gate test: mutation report: .*$HUB/$slug/" "$f" | sed 's/^gate test: mutation report: //; s/; written to .*//')
  echo "$(basename "$f") wall=$wall unreached=$unreached triage=$triage failed=[$failed] mutation=[$mutation]"
done
