#!/usr/bin/env bash
# Usage: latency.sh <out-dir>
# Reads the Claude Code transcripts of the slugs in slugs.txt (main sessions and fork sub-agents) under
# ~/.claude/projects and writes <out-dir>/latency.tsv, one row per Read, Grep, Glob, Edit and Write call that has a result:
# slug session kind role tool ctx ncalls latency_ms
# ctx is the context of the call's message (input + cache write + cache read) and ncalls its call count;
# latency_ms is the time from the call's transcript line to its result's line.
# A result line follows the call's after-tool-call hooks, so the latency holds the time the batching hook took.
# Sessions that started before CUTOFF (default 2026-11-07T13:15:00) are left out.
set -euo pipefail
out=$1
cutoff=${CUTOFF:-2026-11-07T13:15:00}
projects=${PROJECTS:-$HOME/.claude/projects}
mkdir -p "$out"
printf 'slug\tsession\tkind\trole\ttool\tctx\tncalls\tlatency_ms\n' > "$out/latency.tsv"

# shellcheck disable=SC2016
jqprog='
def ms: (.[0:19] + "Z" | fromdateiso8601) * 1000 + ((.[20:23] // "0") | tonumber);
. as $all
| [$all[] | select(.type == "user") | (.timestamp as $ts | (.message.content | arrays | .[]) | select(.type == "tool_result") | {key: .tool_use_id, value: $ts})]
  | from_entries as $resultat
| [$all[] | select(.type == "assistant" and .message.id != null)]
| group_by(.message.id)
| map(
    . as $recs
    | ($recs | map(.message.usage // {}) | map((.input_tokens // 0) + (.cache_creation_input_tokens // 0) + (.cache_read_input_tokens // 0)) | max) as $ctx
    | ([$recs[] | .timestamp as $ts | (.message.content | arrays | .[]) | select(.type == "tool_use") | {id, name, ts: $ts}]) as $calls
    | $calls[] | select(.name | IN("Read", "Grep", "Glob", "Edit", "Write"))
    | select($resultat[.id] != null)
    | [.name, $ctx, ($calls | length), (($resultat[.id] | ms) - (.ts | ms))]
  )
| .[] | @tsv'

process() { # file slug session kind role
  local file=$1 slug=$2 session=$3 kind=$4 role=$5
  jq -rs "$jqprog" "$file" |
    awk -F'\t' -v OFS='\t' -v s="$slug" -v se="$session" -v k="$kind" -v r="$role" '{ print s, se, k, r, $0 }' >> "$out/latency.tsv"
}

while read -r slug; do
  dir=$projects/-home-knatte-Code-loomyard-LYMHUB-$slug
  [ -d "$dir" ] || continue
  for f in "$dir"/*.jsonl; do
    [ -e "$f" ] || continue
    first=$(jq -rn 'first(inputs | select(.type == "user" or .type == "assistant") | .timestamp)' "$f")
    [[ "$first" < "$cutoff" ]] && continue
    title=$(jq -r 'select(.type == "custom-title") | .customTitle' "$f" | tail -1)
    role=${title##*:}; role=${role//\"/}; [ -n "$role" ] || role=untitled
    sid=$(basename "$f" .jsonl)
    process "$f" "$slug" "$sid" main "$role"
    for sub in "$dir/$sid"/subagents/*.jsonl; do
      [ -e "$sub" ] || continue
      process "$sub" "$slug" "$(basename "$sub" .jsonl)" fork "$role+fork"
    done
  done
done < "$(dirname "${BASH_SOURCE[0]}")/slugs.txt"
