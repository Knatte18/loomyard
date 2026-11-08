#!/usr/bin/env bash
# Usage: extract-turns.sh <slugs-file> <out-dir>
# Reads the Claude Code transcripts of the darn runs named in <slugs-file> (one slug a line) under ~/.claude/projects and writes
# <out-dir>/sessions.tsv, turns.tsv, calls.tsv and prompts.tsv for the darn writer sessions (role darn), the cold gate fixer
# sessions (role gate-fixer), and the fork subagents of both.
# sessions.tsv: slug session role kind first_ts last_ts model title_effort turns
# turns.tsv:    slug session kind role turn ts_first ts_last model effort input cache_write_5m cache_write_1h cache_read output thinking_blocks
#               ncalls stop_reason
#   A turn is one assistant message id; ts_first and ts_last are epoch seconds of its earliest and latest streamed record.
#   effort is the assistant record's own effort field.
# calls.tsv:    slug session kind role turn call tool class result_secs capped failed wait key
#   class is gate (`lym gate`), test-lint (go test, vet, build, lint, fmt, validate, drift), read, search, edit, agent, skill, bash or other.
#   result_secs is the tool result's timestamp less the turn's ts_last, empty when no result was recorded.
#   capped is 1 when the result text holds `"done": false` or `done: false`, the capped gate call that asks to be called again.
#   failed is 1 when the result text opens "Exit code" with a non-zero code.
#   wait is 1 when the Bash command holds `--wait`, which asks a `lym gate` call to run to its end instead of at the keep-warm cap.
#   key is the first 120 characters of the Bash command, or the file path or pattern of another tool.
# prompts.tsv:  slug session kind role ts chars head
#   One row per user message that is a real prompt (typed or injected text, not a tool result or meta record).
# Environment: PROJECTS names the transcript root (default ~/.claude/projects), HUB_PREFIX the project directory prefix
# (default -home-knatte-Code-loomyard-LYMHUB-).
# Needs bash and jq; reads only, apart from the tables.
set -euo pipefail
slugs=$1
out=$2
projects=${PROJECTS:-$HOME/.claude/projects}
prefix=${HUB_PREFIX:--home-knatte-Code-loomyard-LYMHUB-}
mkdir -p "$out"
printf 'slug\tsession\trole\tkind\tfirst_ts\tlast_ts\tmodel\ttitle\tturns\n' > "$out/sessions.tsv"
printf 'slug\tsession\tkind\trole\tturn\tts_first\tts_last\tmodel\teffort\tinput\tcache_write_5m\tcache_write_1h\tcache_read\toutput\tthinking_blocks\tncalls\tstop_reason\n' > "$out/turns.tsv"
printf 'slug\tsession\tkind\trole\tturn\tcall\ttool\tclass\tresult_secs\tcapped\tfailed\twait\tkey\n' > "$out/calls.tsv"
printf 'slug\tsession\tkind\trole\tts\tchars\thead\n' > "$out/prompts.tsv"

# shellcheck disable=SC2016
jqprog='
def secs: (.[0:19] + "Z" | fromdateiso8601) + ((.[20:23] | tonumber) / 1000);
def restext: (.content | if type == "array" then map(.text? // "") | join(" ") else (. // "") | tostring end);
def classify($tool; $cmd):
  if $tool == "Read" then "read"
  elif $tool == "Grep" or $tool == "Glob" then "search"
  elif $tool == "Edit" or $tool == "Write" or $tool == "MultiEdit" or $tool == "NotebookEdit" then "edit"
  elif $tool == "Agent" or $tool == "Task" then "agent"
  elif $tool == "Skill" then "skill"
  elif $tool == "Bash" then
    (if ($cmd | test("lym gate")) then "gate"
     elif ($cmd | test("go (test|vet|build)|golangci|shellcheck|lym loom fmt|gofmt|lym loom validate|lym drift|\\blint\\b")) then "test-lint"
     else "bash" end)
  else "other" end;

. as $all
| ([$all[] | select(.type == "user") | .timestamp as $ts | (.message.content | arrays | .[]) | select(.type == "tool_result")
    | {key: .tool_use_id, value: {ts: ($ts | secs), text: restext}}] | from_entries) as $res
| ([$all[] | select(.type == "assistant" and .message.id != null)]
   | group_by(.message.id) | map(sort_by(.timestamp)) | sort_by(.[0].timestamp) | to_entries
   | map(.key as $i | .value as $recs
     | {i: ($i + 1), first: ($recs[0].timestamp | secs), last: ($recs[-1].timestamp | secs),
        model: ($recs[0].message.model // ""), effort: ($recs[0].effort // ""),
        stop: ([$recs[] | .message.stop_reason | select(. != null)] | last // ""),
        thinking: ([$recs[] | .message.content[]? | select(.type == "thinking")] | length),
        calls: ([$recs[] | .message.content[]? | select(.type == "tool_use")] | unique_by(.id)),
        usage: ($recs | map(.message.usage) | max_by(.output_tokens // 0))})) as $turns
| ($turns[] as $t
   | ($t.usage // {}) as $u
   | "T\t\($t.i)\t\($t.first)\t\($t.last)\t\($t.model)\t\($t.effort)\t\($u.input_tokens // 0)\t\($u.cache_creation.ephemeral_5m_input_tokens // $u.cache_creation_input_tokens // 0)\t\($u.cache_creation.ephemeral_1h_input_tokens // 0)\t\($u.cache_read_input_tokens // 0)\t\($u.output_tokens // 0)\t\($t.thinking)\t\($t.calls | length)\t\($t.stop)",
     ($t.calls | to_entries[] | .key as $c | .value as $call
      | (($call.input.command // "") | tostring | gsub("[\\t\\n\\r]+"; " ")) as $cmd
      | (($call.input.file_path // $call.input.pattern // $call.input.path // "") | tostring | gsub("[\\t\\n\\r]+"; " ")) as $fp
      | ($res[$call.id] // null) as $r
      | "C\t\($t.i)\t\($c + 1)\t\($call.name)\t\(classify($call.name; $cmd))\t\(if $r == null then "" else ($r.ts - $t.last) end)\t\(if $r != null and ($r.text | test("\"?done\"?: ?false")) then 1 else 0 end)\t\(if $r != null and ($r.text | test("^Exit code [1-9]")) then 1 else 0 end)\t\(if ($cmd | test("--wait")) then 1 else 0 end)\t\((if $call.name == "Bash" then $cmd else $fp end)[0:120])")),
  ($all[] | select(.type == "user" and (.isMeta | not) and .timestamp != null)
   | (.message.content | if type == "string" then . else (arrays | map(select(.type == "text") | (.text // "")) | join(" ")) end) as $txt
   | select(($txt | length) > 0)
   | "P\t\(.timestamp | secs)\t\($txt | length)\t\($txt[0:200] | gsub("[\\t\\n\\r]+"; " "))")'

process() { # file slug session kind role
  local file=$1 slug=$2 session=$3 kind=$4 role=$5 title=$6
  jq -c 'select(.type == "assistant" or .type == "user")' "$file" | jq -rs "$jqprog" |
    awk -F'\t' -v OFS='\t' -v s="$slug" -v se="$session" -v k="$kind" -v r="$role" -v td="$out/turns.tsv" -v cd="$out/calls.tsv" -v pd="$out/prompts.tsv" '
      $1 == "T" { print s, se, k, r, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14 >> td }
      $1 == "C" { print s, se, k, r, $2, $3, $4, $5, $6, $7, $8, $9, $10 >> cd }
      $1 == "P" { print s, se, k, r, $2, $3, $4 >> pd }'
  jq -rs --arg s "$slug" --arg se "$session" --arg r "$role" --arg k "$kind" --arg t "$title" '
    [.[] | select(.type == "assistant" and .message.id != null)] as $a
    | [$s, $se, $r, $k, ($a | map(.timestamp) | min // ""), ($a | map(.timestamp) | max // ""),
       ($a[0].message.model // ""), $t, ($a | map(.message.id) | unique | length)] | @tsv' "$file" >> "$out/sessions.tsv"
}

while read -r slug; do
  dir=$projects/$prefix$slug
  [ -d "$dir" ] || continue
  for f in "$dir"/*.jsonl; do
    [ -e "$f" ] || continue
    title=$(jq -r 'select(.type == "custom-title") | .customTitle' "$f" | tail -1)
    role=${title##*:}
    case $role in darn | gate-fixer) ;; *) continue ;; esac
    sid=$(basename "$f" .jsonl)
    process "$f" "$slug" "$sid" main "$role" "$title"
    for sub in "$dir/$sid"/subagents/*.jsonl; do
      [ -e "$sub" ] || continue
      process "$sub" "$slug" "$(basename "$sub" .jsonl)" fork "$role+fork" "$title"
    done
  done
done < "$slugs"
