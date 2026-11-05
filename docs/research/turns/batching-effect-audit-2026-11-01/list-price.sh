#!/usr/bin/env bash
# Usage: list-price.sh > prices.tsv
# One row per Plan or Discussion writer session of slugs.txt, main sessions and their fork sub-agents:
# slug session kind role batching cost_usd
# batching is "after" when the main session's transcript holds the Plan/Discussion stencil's batching sentence
# (the stencil text reaches the agent at call time, so its presence in the transcript is the evidence), else "before"; a fork takes its parent's value.
# cost_usd prices each message at the Claude API list price of claude-opus-5-5, the one model every writer session used,
# in USD per million tokens: input 4, cache write 5 (five-minute) or 8 (one-hour), cache read 0.20, output 20.
# These are the opus-5-5 row of internal/shuttleengine/claudeengine/pricing.go; the script fails on any other model.
# A message is the assistant records sharing one message.id; its usage is the record with the most output tokens.
# Environment: PROJECTS names the transcript root (default ~/.claude/projects).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
projects=${PROJECTS:-$HOME/.claude/projects}
sentence='Issue every search and read you can name in one message'

# shellcheck disable=SC2016
cost='
  [ .[] | select(.type == "assistant" and .message.id != null) ]
  | group_by(.message.id)
  | map(max_by(.message.usage.output_tokens // 0))
  | map(
      (.message.model) as $m
      | if $m != "claude-opus-5-5" then error("unpriced model " + $m) else . end
      | .message.usage as $u
      | ($u.cache_creation.ephemeral_1h_input_tokens // 0) as $w1h
      | (($u.cache_creation_input_tokens // 0) - $w1h) as $w5m
      | ( ($u.input_tokens // 0) * 4 + $w5m * 5 + $w1h * 8
          + ($u.cache_read_input_tokens // 0) * 0.20 + ($u.output_tokens // 0) * 20 ) / 1e6 )
  | add // 0'

printf 'slug\tsession\tkind\trole\tbatching\tcost_usd\n'
while read -r slug; do
  dir=$projects/-home-knatte-Code-loomyard-LYMHUB-$slug
  for f in "$dir"/*.jsonl; do
    title=$(jq -r 'select(.type == "custom-title") | .customTitle' "$f" | tail -1)
    role=${title##*:}
    [[ $role == plan || $role == discussion ]] || continue
    sid=$(basename "$f" .jsonl)
    batching=before
    grep -q "$sentence" "$f" && batching=after
    printf '%s\t%s\tmain\t%s\t%s\t%.4f\n' "$slug" "$sid" "$role" "$batching" "$(jq -s "$cost" "$f")"
    for sub in "$dir/$sid"/subagents/*.jsonl; do
      [ -e "$sub" ] || continue
      printf '%s\t%s\tfork\t%s+fork\t%s\t%.4f\n' "$slug" "$(basename "$sub" .jsonl)" "$role" "$batching" "$(jq -s "$cost" "$sub")"
    done
  done
done < "$here/slugs.txt"
