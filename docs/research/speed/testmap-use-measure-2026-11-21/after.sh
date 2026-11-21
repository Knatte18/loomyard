#!/usr/bin/env bash
# Usage: after.sh <slug> <session-id-prefix> <timestamp>
# Prints what the session did from the timestamp to its next typed user message:
# the files it edited, the commit commands it ran, the subagents it started and its text replies.
# Then prints the stretch's cost: seconds, model requests, output and cache-read tokens.
# Needs bash, jq, awk and GNU date; reads only.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
f=$(session "$1" "$2")
jq -r --arg t "$3" '
  select(.timestamp > $t)
  | if .type=="user" and (.message.content|type=="string") then "USER \(.timestamp) \(.message.content[0:120])"
    elif .type=="assistant" then (.message.content[]? |
      if .type=="tool_use" and (.name=="Edit" or .name=="Write") then "EDIT \(.input.file_path)"
      elif .type=="tool_use" and .name=="Bash" and (.input.command|test("git commit")) then "COMMIT \(.input.command[0:200])"
      elif .type=="tool_use" and .name=="Agent" then "AGENT \(.input.description)"
      elif .type=="text" then "TEXT \(.text[0:600])"
      else empty end)
    else empty end' "$f" | awk '/^USER/ { n++ } n < 2 { print }'
jq -r --arg t "$3" '
  select(.timestamp > $t)
  | if .type=="user" and (.message.content|type=="string") then "U \(.timestamp)"
    elif .type=="assistant" then "A \(.timestamp) \(.message.id) \(.message.usage.output_tokens // 0) \(.message.usage.cache_read_input_tokens // 0)"
    else empty end' "$f" |
  awk -v start="$3" '
    function ep(t,   c, s) { c = "date -d " t " +%s"; c | getline s; close(c); return s }
    $1 == "U" { if (++u >= 2) exit; next }
    { last = $2; if (!($3 in seen)) { seen[$3] = 1; n++; out += $4; read += $5 } }
    END { printf "COST seconds=%d requests=%d output=%d cache_read=%d\n", ep(last) - ep(start "Z"), n, out, read }'
