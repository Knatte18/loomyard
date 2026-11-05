#!/usr/bin/env bash
# forkcheck.sh <run-dir>...: per fork of each harvested run, the tool calls it made, any Write/Edit/NotebookEdit/Agent call,
# any git command other than a read-only one, and whether the fixer spawned its forks in one message.
for R in "$@"; do
  echo "== $(basename "$R")"
  for m in "$R"/transcripts/*/subagents/*.meta.json; do
    [ -f "$m" ] || continue
    g=${m%.meta.json}.jsonl
    jq -s -r --arg d "$(jq -r .description "$m")" '
      # The first assistant message of a fork transcript is its copy of the fixer'"'"'s spawn message, so it is left out.
      ([ .[] | select(.type=="assistant") ][0].message.id) as $spawn
      | [ .[] | select(.type=="assistant" and .message.id != $spawn) | .message.content[]? | select(.type=="tool_use") ] as $t
      | ($t | map(select(.name=="Write" or .name=="Edit" or .name=="NotebookEdit" or .name=="Agent")) | length) as $w
      | ($t | map(select(.name=="Bash") | .input.command) | map(select(test("\\bgit\\b") and (test("\\bgit (-C [^ ]+ )?(log|show|diff|status|grep|ls-files|rev-parse|blame|cat-file)\\b") | not))) | length) as $g
      | [$d, ($t|length), $w, $g, ($t | map(.name + ":" + ((.input.file_path // .input.pattern // .input.command // "") | tostring | .[0:60])) | join(" | "))] | @tsv' "$g"
  done
  for f in "$R"/transcripts/*.jsonl; do
    jq -r 'select(.type=="assistant") | [.message.id, ([.message.content[]? | select(.type=="tool_use" and .name=="Agent")] | length)] | @tsv' "$f" | awk '$2>0 {n[$1]+=$2} END {for (k in n) print "spawn message", k, n[k], "forks"}'
  done
done
