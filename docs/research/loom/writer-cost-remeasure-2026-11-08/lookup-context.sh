#!/usr/bin/env bash
# Usage: lookup-context.sh <slugs-file> <cutoff> >> lookup-context.tsv
# For each lookup subagent (a general-purpose Agent call) of the Plan-Write and Discussion-Write main sessions that started on or after <cutoff>
# (an ISO timestamp or date, compared as text), writes the context the subagent read in:
#   slug session role batch description first_ctx last_ctx
# first_ctx is the context of the subagent's first turn (its prompt and the fixed system text) and last_ctx that of its last turn;
# last_ctx minus first_ctx is the lookup context the subagent took in, an upper bound on what the writer would have kept had it read the same itself.
# A subagent with no transcript has no row. Needs bash and jq; PROJECTS names the transcript root (default ~/.claude/projects).
set -euo pipefail
slugs=$1
cutoff=$2
projects=${PROJECTS:-$HOME/.claude/projects}

# batch number, tool use id and description of each lookup Agent call of one main transcript
# shellcheck disable=SC2016
callsprog='
[.[] | select(.type == "assistant" and .message.id != null)] | group_by(.message.id) | map(sort_by(.timestamp)) | sort_by(.[0].timestamp)
| map([.[] | .message.content[]? | select(.type == "tool_use" and (.name == "Agent" or .name == "Task"))] | unique_by(.id))
| map(select(length > 0)) | to_entries[] | (.key + 1) as $b | .value[]
| select((.input.subagent_type // "general-purpose") == "general-purpose")
| [$b, .id, (.input.description // "")] | @tsv'

# context of the first and last turn of one subagent transcript
# shellcheck disable=SC2016
ctxprog='
[.[] | select(.type == "assistant" and .message.id != null)] | group_by(.message.id) | map(max_by(.message.usage.output_tokens // 0)) | sort_by(.timestamp)
| map(.message.usage | (.input_tokens // 0) + (.cache_creation_input_tokens // 0) + (.cache_read_input_tokens // 0))
| [.[0], .[-1]] | @tsv'

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
    jq -rs "$callsprog" "$f" | while IFS=$'\t' read -r batch id description; do
      for meta in "$dir/$sid"/subagents/*.meta.json; do
        [ -e "$meta" ] || continue
        [ "$(jq -r .toolUseId "$meta")" = "$id" ] || continue
        sub=${meta%.meta.json}.jsonl
        [ -e "$sub" ] || break
        IFS=$'\t' read -r firstctx lastctx < <(jq -rs "$ctxprog" "$sub")
        printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$sid" "$role" "$batch" "$description" "$firstctx" "$lastctx"
        break
      done
    done
  done
done < "$slugs"
