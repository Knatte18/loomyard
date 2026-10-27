#!/usr/bin/env bash
# evidence.sh <run-cell-dir>: one line per rollout of a run: id, parent, start, end, own commands, first request and last total, weekly window.
# A fork's own commands are its custom_tool_call count less the handler's calls made before the fork's start.
cell=$1
handler=$(grep -L forked_from_id "$cell"/rollouts/*.jsonl | head -1)
ncalls() { grep -c '"payload":{"type":"custom_tool_call"' "$1"; }
first_ts() { head -1 "$1" | grep -o '"timestamp":"[^"]*"' | head -1 | cut -d'"' -f4; }
last_ts() { tail -1 "$1" | grep -o '"timestamp":"[^"]*"' | head -1 | cut -d'"' -f4; }
tot() { grep '"token_count"' "$1" | tail -1 | grep -o '"total_token_usage":{[^}]*}'; }
first() { grep '"token_count"' "$1" | grep -o '"last_token_usage":{[^}]*}' | head -1; }
for f in "$cell"/rollouts/*.jsonl; do
  id=$(basename "$f" .jsonl | cut -c37-)
  parent=$(head -c 4000 "$f" | grep -o '"forked_from_id":"[^"]*"' | head -1 | cut -d'"' -f4)
  start=$(first_ts "$f")
  n=$(ncalls "$f")
  if [ -n "$parent" ]; then
    inherited=$(awk -v s="$start" '/"payload":{"type":"custom_tool_call"/ { match($0,/"timestamp":"[^"]*"/); t=substr($0,RSTART+13,RLENGTH-14); if (t < s) c++ } END {print c+0}' "$handler")
    own=$((n-inherited))
  else own=$n; inherited=0; fi
  echo "id=$id forked_from=${parent:--} start=$start end=$(last_ts "$f") commands_total=$n inherited=$inherited own_commands=$own first_request=$(first "$f") total=$(tot "$f")"
done
