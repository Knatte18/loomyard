#!/usr/bin/env bash
# Usage: extract-turns.sh <out-dir>
# Reads the Claude Code transcripts of the slugs in slugs.txt (main sessions and fork sub-agents) under
# ~/.claude/projects and writes <out-dir>/turns.tsv and <out-dir>/calls.tsv.
# turns.tsv: slug session kind role turn ts ncalls ctx input cache_write cache_read output weight dep
# calls.tsv: slug session kind role turn call ts tool class key
# A turn is one assistant message id; ctx is input + cache write + cache read;
# weight is input + 1.25 x cache write + 0.1 x cache read + 5 x output.
# dep is 1 when a path-like token of the turn's calls appears in the previous turn's tool results and nowhere earlier in the session
# (prompt, attachments, older results, earlier call inputs): the call could not have been issued a turn earlier.
# Sessions that started before CUTOFF (default 2026-10-20) are left out.
set -euo pipefail
out=$1
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cutoff=${CUTOFF:-2026-10-20}
projects=${PROJECTS:-$HOME/.claude/projects}
mkdir -p "$out"
printf 'slug\tsession\tkind\trole\tturn\tts\tncalls\tctx\tinput\tcache_write\tcache_read\toutput\tweight\tdep\n' > "$out/turns.tsv"
printf 'slug\tsession\tkind\trole\tturn\tcall\tts\ttool\tclass\tkey\n' > "$out/calls.tsv"

# shellcheck disable=SC2016
jqprog='
def segwords($cmd):
  [ $cmd
    | gsub("\"[^\"]*\""; "Q") | gsub("\u0027[^\u0027]*\u0027"; "Q")
    | gsub("[0-9]*>&[0-9]"; "") | gsub("[0-9]*>>?\\s*/dev/null"; "")
    | split("[;&|\\n]+"; "g")[]
    | gsub("^\\s+|\\s+$"; "")
    | gsub("^((for|while|if|then|do|else|elif)\\s+)+"; "")
    | gsub("^([A-Za-z_][A-Za-z0-9_]*=(\"[^\"]*\"|\\S*)\\s*)+"; "")
    | select(length > 0) ];
def classify_bash($cmd):
  segwords($cmd) as $segs
  | ($cmd | gsub("[0-9]*>&[0-9]"; "") | gsub("[0-9]*>>?\\s*/dev/null"; "")) as $c
  | if ($c | test("lym gate")) then "gate"
    elif ($c | test("go (test|vet|build)|golangci|shellcheck|lym loom fmt|gofmt|\\blint\\b")) then "test-lint"
    elif ($c | test("lym loom validate")) then "validate"
    elif ($c | test("sed -i|perl -[0-9a-z]*i|\\btee\\b|<<|(^|\\s)>>?\\s*[^&\\s=]")) then
      (if ($c | test("(>>?\\s*|tee\\s+)(\\S*scratch\\S*|\\$S\\b\\S*|\\$OUT\\S*|\\$\\{?S\\}?\\S*|/tmp\\S*)")) and ($c | test("sed -i|perl -[0-9a-z]*i") | not) then "scratch-write" else "edit-script" end)
    elif ($segs | any(test("^(mv|cp|rm|mkdir|touch|chmod|ln)\\b|^git (add|commit|checkout|merge|rebase|push|reset|stash|restore)\\b"))) then "other-write"
    elif ($segs | any(test("^lym quarry"))) then "quarry"
    elif ($segs | all(test("^(cd|echo|printf|pwd|true|test|\\[|done|fi|basename|dirname|export|set|sleep|wait)\\b") or test("^(cat|head|tail|sed|nl|wc|cut|sort|uniq|column|tr|diff|jq|stat|file|git( -C \\S+)? (status|log|diff|show|rev-parse|branch|ls-files|blame|cat-file|merge-base)|lym (board (get|list)|loom (peers|show)|[a-z]+ (status|list|show|get))|grep|rg|find|ls|tree|awk|xargs|readlink|realpath|which|type|env|date|head)\\b"))) then
      (if ($segs | any(test("^git( -C \\S+)? "))) then "git-read"
       elif ($segs | any(test("^lym "))) then "lym-read"
       elif ($segs | any(test("^(grep|rg|find|ls|tree|awk)\\b"))) then "search"
       else "read" end)
    else "other-bash" end;
def classify($tool; $cmd):
  if $tool == "Read" then "read"
  elif $tool == "Grep" or $tool == "Glob" then "search"
  elif $tool == "Edit" or $tool == "Write" or $tool == "MultiEdit" or $tool == "NotebookEdit" then "edit"
  elif $tool == "Bash" then classify_bash($cmd)
  elif $tool == "Skill" then "skill"
  elif $tool == "SendMessage" then "message"
  elif $tool == "Agent" or $tool == "Task" then "agent"
  else "other" end;
def toks($s): [$s | scan("[A-Za-z0-9_./-]*[/.][A-Za-z0-9_./-]+") | select(length >= 6)] | unique;
def restext: (.content | if type == "array" then map(.text? // "") | join(" ") else (. // "") | tostring end);

. as $all
| [$all[] | select(.type == "user") | (.message.content | arrays | .[]) | select(.type == "tool_result")
   | {key: .tool_use_id, value: restext}] | from_entries as $res
| ([$all[] | select(.type == "user") | .message.content | if type == "string" then . else (arrays | map(select(.type == "text") | .text) | join(" ")) end] | join(" ")) as $prompt
| ([$all[] | select(.type == "attachment") | .attachment | tostring] | join(" ")) as $attached
| [$all[] | select(.type == "assistant" and .message.id != null)]
| group_by(.message.id)
| map(sort_by(.timestamp))
| sort_by(.[0].timestamp)
| to_entries
| map(.key as $i | .value as $recs
  | {i: ($i + 1), ts: $recs[0].timestamp,
     calls: ([$recs[] | .message.content[]? | select(.type == "tool_use")] | unique_by(.id)),
     usage: ($recs | map(.message.usage) | max_by(.output_tokens // 0))})
| . as $turns
| range(0; length) as $n
| $turns[$n] as $t
| ($t.usage // {}) as $u
| (($u.input_tokens // 0)) as $in
| (($u.cache_creation_input_tokens // 0)) as $cw
| (($u.cache_read_input_tokens // 0)) as $cr
| (($u.output_tokens // 0)) as $o
| (if $n == 0 then ""
   else ([$turns[$n - 1].calls[] | $res[.id] // ""] | join(" ")) end) as $prev
| ([$t.calls[] | toks((.input.command // .input.file_path // .input.pattern // .input.path // "") | tostring)] | add // []) as $tk
| ($prompt + " " + $attached + " " + ([$turns[0:(if $n > 1 then $n - 1 else 0 end)][].calls[] | ($res[.id] // "")] | join(" ")) + " " + ([$turns[0:$n][].calls[] | (.input | tostring)] | join(" "))) as $known
| (if ($tk | length) > 0 and ($prev | length) > 0 and ([$tk[] | select(. as $tok | ($prev | contains($tok)) and (($known | contains($tok)) | not))] | length) > 0 then 1 else 0 end) as $dep
| (
  "T\t\($t.i)\t\($t.ts)\t\($t.calls | length)\t\($in + $cw + $cr)\t\($in)\t\($cw)\t\($cr)\t\($o)\t\($in + 1.25 * $cw + 0.1 * $cr + 5 * $o)\t\($dep)",
  ($t.calls | to_entries[] | .key as $c | .value as $call
   | (($call.input.command // "") | tostring | gsub("[\\t\\n\\r]+"; " ")) as $cmd
   | (($call.input.file_path // $call.input.pattern // $call.input.path // "") | tostring | gsub("[\\t\\n\\r]+"; " ")) as $fp
   | "C\t\($t.i)\t\($c + 1)\t\($t.ts)\t\($call.name)\t\(classify($call.name; $cmd))\t\((if $call.name == "Bash" then $cmd else $fp end)[0:160])")
)'

process() { # file slug session kind role
  local file=$1 slug=$2 session=$3 kind=$4 role=$5
  jq -rs "$jqprog" "$file" | awk -F'\t' -v OFS='\t' -v s="$slug" -v se="$session" -v k="$kind" -v r="$role" -v td="$out/turns.tsv" -v cd="$out/calls.tsv" '
    $1 == "T" { print s, se, k, r, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11 >> td }
    $1 == "C" { print s, se, k, r, $2, $3, $4, $5, $6, $7 >> cd }'
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
done < "$here/slugs.txt"
