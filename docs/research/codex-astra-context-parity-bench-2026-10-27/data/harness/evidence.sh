#!/usr/bin/env bash
# evidence.sh <cell>: print what the checks on a fork-fan cell need, from the rollouts it copied.
# Per rollout: id, parent, start, end, the commands run after the thread's own task start, the first model request and the last token_count.
# Then the handler's spawn_agent calls and whether the handler's rollout carries the AGENTS.md instructions message.
set -uo pipefail
cell=$1
echo "# One line per rollout of the run: start, end, parent, commands, first request, total tokens"
for f in "$cell"/rollouts/rollout-*.jsonl; do
  id=$(head -1 "$f" | jq -r '.payload.id')
  parent=$(head -1 "$f" | jq -r '.payload.forked_from_id // "-"')
  start=$(head -1 "$f" | jq -r '.payload.timestamp')
  end=$(tail -1 "$f" | jq -r '.timestamp')
  own_start=$(jq -r 'select(.type=="event_msg" and .payload.type=="task_started") | .timestamp' "$f" | tail -1)
  commands=$(jq -r --arg s "$own_start" 'select(.type=="response_item" and .payload.type=="custom_tool_call" and .timestamp >= $s) | 1' "$f" | wc -l)
  first=$(jq -c 'select(.type=="event_msg" and .payload.type=="token_count") | .payload.info.last_token_usage | {input_tokens,cached_input_tokens}' "$f" | head -1)
  total=$(jq -c 'select(.type=="event_msg" and .payload.type=="token_count") | .payload.info.total_token_usage' "$f" | tail -1)
  echo "id=$id forked_from=$parent start=$start end=$end commands=$commands first_request=$first total=$total"
done
echo
echo "# spawn_agent calls in the handler's rollout (message bodies are encrypted in the rollout)"
handler=$(for f in "$cell"/rollouts/rollout-*.jsonl; do head -1 "$f" | jq -e '.payload.forked_from_id == null' >/dev/null && echo "$f"; done)
jq -r 'select(.type=="response_item" and .payload.type=="function_call" and .payload.name=="spawn_agent") | "\(.timestamp)\t\(.payload.arguments)"' "$handler" | while IFS=$'\t' read -r ts args; do
  echo "$ts spawn_agent task_name=$(jq -r .task_name <<<"$args") fork_turns=$(jq -r .fork_turns <<<"$args")"
done
echo
echo "# AGENTS.md instructions message in the handler's rollout (ordinal, role, first 120 characters)"
jq -r 'select(.type=="response_item" and .payload.type=="message") | .payload as $p | ($p.content[0].text // "") | select(startswith("# AGENTS.md instructions")) | "\(.[0:120])"' "$handler" | head -1
jq -r 'select(.type=="response_item" and .payload.type=="message" and ((.payload.content[0].text // "") | startswith("# AGENTS.md instructions"))) | "ordinal=\(.ordinal) role=\(.payload.role) characters=\(.payload.content[0].text | length)"' "$handler"
