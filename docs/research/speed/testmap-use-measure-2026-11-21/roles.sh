#!/usr/bin/env bash
# Usage: roles.sh <slug>
# Prints one line per session of the run: its id, first and last timestamp, and the first heading of the prompt it read.
# Needs bash and jq; reads only.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
for f in "$(sessions "$1")"/*.jsonl; do
  start=$(jq -r 'select(.timestamp) | .timestamp' "$f" | head -1)
  end=$(jq -r 'select(.timestamp) | .timestamp' "$f" | tail -1)
  role=$(jq -r 'select(.type=="user") | .message.content | arrays | .[] | select(.type=="tool_result")
    | .content | if type=="array" then (.[0].text // "") else . end' "$f" 2>/dev/null | grep -m1 -o '^ *1.# .*' | cut -c1-90)
  echo "$(basename "$f" .jsonl) $start $end $role"
done | sort -k2
