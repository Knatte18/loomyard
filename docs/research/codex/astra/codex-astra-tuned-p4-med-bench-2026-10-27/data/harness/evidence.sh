#!/usr/bin/env bash
# evidence.sh <rollouts-dir>: print the run's per-rollout figures, the handler's spawn_agent calls, the AGENTS.md loads and the pattern/ reads before the first spawn.
set -uo pipefail
dir=$1
echo "# One line per rollout: id, parent, start, end, commands, first model request, last total, window after"
for f in "$dir"/*.jsonl; do
  id=$(jq -r 'select(.type=="session_meta")|.payload.id' "$f" | head -1)
  parent=$(jq -r 'select(.type=="session_meta")|.payload.forked_from_id // "-"' "$f" | head -1)
  start=$(jq -r '.timestamp' "$f" | head -1); end=$(jq -r '.timestamp' "$f" | tail -1)
  cmds=$(jq -c 'select(.type=="response_item" and .payload.type=="custom_tool_call" and .payload.name=="exec")' "$f" | wc -l)
  first=$(jq -c 'select(.type=="event_msg" and .payload.type=="token_count")|.payload.info.last_token_usage|{input_tokens,cached_input_tokens}' "$f" | head -1)
  total=$(jq -c 'select(.type=="event_msg" and .payload.type=="token_count")|.payload.info.total_token_usage|{input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens}' "$f" | tail -1)
  win=$(jq -r 'select(.type=="event_msg" and .payload.type=="token_count")|.payload.rate_limits.primary.used_percent' "$f" | tail -1)
  echo "id=$id forked_from=$parent start=$start end=$end commands_including_inherited=$cmds first_request=$first total=$total weekly_used_percent_end=$win"
done
echo
echo "# spawn_agent calls in the handler's rollout (message bodies are encrypted)"
h=$(grep -L '"forked_from_id":"' "$dir"/*.jsonl | head -1)
jq -r 'select(.type=="response_item" and .payload.type=="function_call" and .payload.name=="spawn_agent")|[.timestamp,(.payload.arguments|fromjson|{task_name,fork_turns}|tojson)]|@tsv' "$h"
echo
echo "# AGENTS.md load notices (project instructions in the first model input), per rollout"
for f in "$dir"/*.jsonl; do echo "$(basename "$f" | cut -c9-60) $(grep -c 'AGENTS.md instructions for' "$f")"; done
