#!/usr/bin/env bash
# Usage: plan-reads.sh <slugs-file> <cutoff> >> plan-reads.tsv
# For each Plan-Write main session that started on or after <cutoff> (an ISO timestamp or date, compared as text), writes the size of the lookup results
# the writer itself took in, by the kind of call:
#   slug session reads_whole reads_whole_tokens reads_slice reads_slice_tokens searches searches_tokens bash bash_tokens
# reads_whole counts Read calls with neither offset nor limit; reads_slice those with either. searches are Grep and Glob calls; bash is every Bash call.
# A result's tokens are its characters divided by 4, an estimate, summed over the session; results of calls made inside a subagent are not in the main transcript.
# Needs bash and jq; PROJECTS names the transcript root (default ~/.claude/projects).
set -euo pipefail
slugs=$1
cutoff=$2
projects=${PROJECTS:-$HOME/.claude/projects}

# shellcheck disable=SC2016
prog='
def restext: (.content | if type == "array" then map(.text? // "") | join("") else (. // "") | tostring end);
. as $all
| [$all[] | select(.type == "user") | (.message.content | arrays | .[]) | select(.type == "tool_result") | {key: .tool_use_id, value: ((restext | length) / 4)}] | from_entries as $size
| [$all[] | select(.type == "assistant") | .message.content[]? | select(.type == "tool_use")] | unique_by(.id)
| map(. as $c | {kind: (if $c.name == "Read" then (if ($c.input.offset != null or $c.input.limit != null) then "slice" else "whole" end)
                         elif $c.name == "Grep" or $c.name == "Glob" then "search"
                         elif $c.name == "Bash" then "bash" else "other" end), tokens: ($size[$c.id] // 0)})
| group_by(.kind) | map({key: .[0].kind, value: {n: length, tokens: (map(.tokens) | add)}}) | from_entries
| [(.whole.n // 0), ((.whole.tokens // 0) | floor), (.slice.n // 0), ((.slice.tokens // 0) | floor), (.search.n // 0), ((.search.tokens // 0) | floor), (.bash.n // 0), ((.bash.tokens // 0) | floor)] | @tsv'

while read -r slug; do
  dir=$projects/-home-knatte-Code-loomyard-LYMHUB-$slug
  [ -d "$dir" ] || continue
  for f in "$dir"/*.jsonl; do
    [ -e "$f" ] || continue
    first=$(jq -rn 'first(inputs | select(.type == "user" or .type == "assistant") | .timestamp)' "$f")
    [[ "$first" < "$cutoff" ]] && continue
    title=$(jq -r 'select(.type == "custom-title") | .customTitle' "$f" | tail -1)
    [ "${title##*:}" = plan\" ] || [ "${title##*:}" = plan ] || continue
    printf '%s\t%s\t%s\n' "$slug" "$(basename "$f" .jsonl)" "$(jq -rs "$prog" "$f")"
  done
done < "$slugs"
