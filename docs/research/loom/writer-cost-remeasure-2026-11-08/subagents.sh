#!/usr/bin/env bash
# Usage: subagents.sh <out-dir>
# Reads the Plan-Write and Discussion-Write main transcripts of the slugs in slugs.txt (sessions that started on or after CUTOFF, default 2026-11-05)
# and the sub-agents each started, and writes <out-dir>/writer-subagents.tsv, one row per Agent call:
# slug session role batch kind description model questions turns cost_usd
#   batch numbers the writer's messages that hold Agent calls, in order, so rows with the same batch were started together.
#   kind is lookup for a general-purpose sub-agent and fork for a fork (the writer, the lens reviewers); only lookup rows are lookups.
#   questions counts the numbered or bulleted lines of the call's prompt, an estimate of the lookups the sub-agent was handed.
#   turns and cost_usd come from the sub-agent's own transcript, priced at its model's list price (internal/shuttleengine/claudeengine/pricing.go);
#   a sub-agent with no transcript has no turns and no cost.
# Needs bash and jq.
set -euo pipefail
out=$1
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cutoff=${CUTOFF:-2026-11-05}
projects=${PROJECTS:-$HOME/.claude/projects}
mkdir -p "$out"
printf 'slug\tsession\trole\tbatch\tkind\tdescription\tmodel\tquestions\tturns\tcost_usd\n' > "$out/writer-subagents.tsv"

# calls of one main transcript: batch, tool use id, subagent type, model, description, question lines
# shellcheck disable=SC2016
callsprog='
[.[] | select(.type == "assistant" and .message.id != null)] | group_by(.message.id) | map(sort_by(.timestamp)) | sort_by(.[0].timestamp)
| map([.[] | .message.content[]? | select(.type == "tool_use" and (.name == "Agent" or .name == "Task"))] | unique_by(.id))
| map(select(length > 0)) | to_entries[] | (.key + 1) as $b | .value[]
| [$b, .id, (.input.subagent_type // "general-purpose"), (.input.model // "inherit"), (.input.description // ""),
   ((.input.prompt // "") | [split("\n")[] | select(test("^\\s*([0-9]+[.)]|[-*]) "))] | length)] | @tsv'

# turns and cost of one sub-agent transcript at its model's list price, in USD per million tokens
# shellcheck disable=SC2016
costprog='
def price($m): {"claude-opus-5-5": [4, 5, 8, 0.2, 20], "claude-sonnet-5-5": [2, 2.5, 4, 0.2, 10]}[$m] // [0, 0, 0, 0, 0];
[.[] | select(.type == "assistant" and .message.id != null)] | group_by(.message.id)
| map(max_by(.message.usage.output_tokens // 0)) as $t
| [($t | length),
   ([$t[] | .message.model as $m | .message.usage as $u | price($m) as $p
     | (($u.input_tokens // 0) * $p[0] + (($u.cache_creation_input_tokens // 0) - ($u.cache_creation.ephemeral_1h_input_tokens // 0)) * $p[1]
        + ($u.cache_creation.ephemeral_1h_input_tokens // 0) * $p[2] + ($u.cache_read_input_tokens // 0) * $p[3] + ($u.output_tokens // 0) * $p[4]) / 1e6] | add // 0)] | @tsv'

while read -r slug; do
  dir=$projects/-home-knatte-Code-loomyard-LYMHUB-$slug
  [ -d "$dir" ] || continue
  for f in "$dir"/*.jsonl; do
    [ -e "$f" ] || continue
    first=$(jq -rn 'first(inputs | select(.type == "user" or .type == "assistant") | .timestamp)' "$f")
    [[ "$first" < "$cutoff" ]] && continue
    title=$(jq -r 'select(.type == "custom-title") | .customTitle' "$f" | tail -1)
    role=${title##*:}; role=${role//\"/}
    [ "$role" = plan ] || [ "$role" = discussion ] || continue
    sid=$(basename "$f" .jsonl)
    jq -rs "$callsprog" "$f" | while IFS=$'\t' read -r batch id type model description questions; do
      kind=fork; [ "$type" = general-purpose ] && kind=lookup
      turns=; cost=; submodel=$model
      for meta in "$dir/$sid"/subagents/*.meta.json; do
        [ -e "$meta" ] || continue
        [ "$(jq -r .toolUseId "$meta")" = "$id" ] || continue
        sub=${meta%.meta.json}.jsonl
        [ -e "$sub" ] && IFS=$'\t' read -r turns cost < <(jq -rs "$costprog" "$sub")
        [ "$kind" = lookup ] && submodel=$(jq -r .model "$meta")
        break
      done
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$sid" "$role" "$batch" "$kind" "$description" "$submodel" "$questions" "$turns" "$cost"
    done >> "$out/writer-subagents.tsv"
  done
done < "$here/slugs.txt"
