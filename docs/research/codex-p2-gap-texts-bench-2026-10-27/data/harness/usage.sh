#!/usr/bin/env bash
# usage.sh <run-cell-dir> <run>: one line per rollout of a run, handler first: wall seconds, tokens, own commands and the weekly used_percent of the first and last token_count.
# The lens of a fork is read from the handler's spawn_agent call order: affected_set, guard_reach, way_forward by start time.
cell=$1 run=$2
handler=$(grep -L forked_from_id "$cell"/rollouts/*.jsonl | head -1)
ncalls() { grep -c '"payload":{"type":"custom_tool_call"' "$1"; }
ts() { grep -o '"timestamp":"[^"]*"' | head -1 | cut -d'"' -f4; }
epoch() { date -u -d "$1" +%s.%N; }
pct() { grep '"token_count"' "$1" | "$2" -1 | grep -o '"primary":{[^}]*}' | grep -o '"used_percent":[0-9.]*' | cut -d: -f2; }
line() { # file label inherited
  local f=$1 label=$2 inh=$3
  local s e w tot in cached out reason n
  s=$(head -1 "$f" | ts); e=$(tail -1 "$f" | ts)
  w=$(echo "$(epoch "$e") - $(epoch "$s")" | bc | cut -d. -f1)
  tot=$(grep '"token_count"' "$f" | tail -1 | grep -o '"total_token_usage":{[^}]*}')
  in=$(grep -o '"input_tokens":[0-9]*' <<<"$tot" | cut -d: -f2); cached=$(grep -o '"cached_input_tokens":[0-9]*' <<<"$tot" | cut -d: -f2)
  out=$(grep -o '"output_tokens":[0-9]*' <<<"$tot" | cut -d: -f2); reason=$(grep -o '"reasoning_output_tokens":[0-9]*' <<<"$tot" | cut -d: -f2)
  n=$(( $(ncalls "$f") - inh ))
  echo "$run $label wall=$w in=$in cached=$cached fresh=$((in-cached)) out=$out reason=$reason commands=$n weekly_first=$(pct "$f" head) weekly_last=$(pct "$f" tail)"
}
line "$handler" handler 0
i=0
for f in $(grep -l forked_from_id "$cell"/rollouts/*.jsonl | xargs -n1 sh -c 'echo "$(head -1 $0 | grep -o "\"timestamp\":\"[^\"]*\"" | head -1) $0"' | sort | cut -d' ' -f2); do
  start=$(head -1 "$f" | ts)
  inh=$(awk -v s="$start" '/"payload":{"type":"custom_tool_call"/ { match($0,/"timestamp":"[^"]*"/); t=substr($0,RSTART+13,RLENGTH-14); if (t < s) c++ } END {print c+0}' "$handler")
  lens=$(sed -n "$((i+1))p" <<<$'affected-set\nguard-reach\nway-forward'); i=$((i+1))
  line "$f" "fork-$lens" "$inh"
done
