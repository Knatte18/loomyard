#!/usr/bin/env bash
# evidence.sh <input-id> <label>: print the rollout evidence and usage lines of one run cell under .scratch/bench/runs/<input-id>/<label>.
# A "command" is one exec call in a rollout; a fork's own count is its total less the handler's calls before the first spawn.
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench
cell=$ROOT/.scratch/bench/runs/$1/$2
cd "$cell/rollouts" || exit 1
handler=$(grep -L '"forked_from_id":"0' * | head -1)
for f in *; do
  jq -r 'select(.type=="session_meta")|.payload|.id' "$f" | grep -q . || continue
done
fid() { jq -r 'select(.type=="session_meta")|.payload.forked_from_id // "-"' "$1" | head -1; }
for f in $(ls); do [ "$(fid $f)" = "-" ] && handler=$f; done
first_spawn=$(jq -r 'select(.type=="response_item" and .payload.name=="spawn_agent")|.timestamp' "$handler" | head -1)
pre=$(jq -r --arg t "$first_spawn" 'select(.type=="response_item" and .payload.name=="exec" and .timestamp<$t)|1' "$handler" | wc -l)
echo "# run $1/$2: id, parent, start, end, wall_s, commands (own), first request, last total, weekly used_percent first/last"
for f in $(ls); do
  id=$(jq -r 'select(.type=="session_meta")|.payload.id' $f | head -1)
  st=$(head -1 $f | jq -r .timestamp)
  en=$(tail -1 $f | jq -r .timestamp)
  wall=$(( $(date -d "$en" +%s) - $(date -d "$st" +%s) ))
  cmds=$(jq -r 'select(.type=="response_item" and .payload.name=="exec")|1' $f | wc -l)
  own=$cmds; [ "$f" != "$handler" ] && own=$((cmds-pre))
  first=$(jq -c 'select(.type=="event_msg" and .payload.type=="token_count" and .payload.info!=null)|.payload.info.last_token_usage|{input_tokens,cached_input_tokens}' $f | head -1)
  tot=$(jq -c 'select(.type=="event_msg" and .payload.type=="token_count" and .payload.info!=null)|.payload.info.total_token_usage|{input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens}' $f | tail -1)
  wf=$(jq -r 'select(.type=="event_msg" and .payload.type=="token_count")|.payload.rate_limits.primary.used_percent' $f | head -1)
  wl=$(jq -r 'select(.type=="event_msg" and .payload.type=="token_count")|.payload.rate_limits.primary.used_percent' $f | tail -1)
  ag=$(jq -r 'select(.type=="response_item")|tostring' $f | grep -c 'AGENTS.md instructions for')
  fresh=$(jq -n --argjson t "$tot" '$t.input_tokens-$t.cached_input_tokens')
  echo "id=$id kind=$([ "$f" = "$handler" ] && echo handler || echo fork) forked_from=$(fid $f) start=$st end=$en wall=$wall commands_own=$own commands_total=$cmds first_request=$first total=$tot fresh=$fresh weekly_first=$wf weekly_last=$wl agents_md_blocks=$ag"
done
echo "# spawn_agent calls in the handler"
jq -r 'select(.type=="response_item" and .payload.name=="spawn_agent")|"\(.timestamp)\t\(.payload.arguments|fromjson|del(.message)|tostring)"' "$handler"
echo "# handler exec calls before the first spawn: $pre"
