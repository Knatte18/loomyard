#!/usr/bin/env bash
# measure.sh breaks the context of one API request in a Claude Code session transcript down by source.
# It rebuilds the request from the transcript's api-request records:
# the shape holds the system prompt and tool schemas, and each request keeps a prefix of the messages before it and adds blobs.
#
# Usage: measure.sh TRANSCRIPT FROM_LINE REQUEST_ORDINAL [SOURCE]
#   FROM_LINE        the transcript line the session start or context cycle begins at (1 for a fresh session)
#   REQUEST_ORDINAL  which request after FROM_LINE to measure, counting from 1
#   SOURCE           print that source's lines instead of the table, to check the sorting
#
# The table holds one row per source: characters, and tokens estimated by splitting the input tokens
# the reply to that request reports in proportion to characters.
# pipefail stays off: head and grep -m 1 end their pipes early on purpose.
set -eu

transcript=$1
from_line=$2
ordinal=$3
show=${4:-}
here=$(cd "$(dirname "$0")" && pwd)

requests=$(tail -n +"$from_line" "$transcript" | grep '"type":"api-request"' | head -n "$ordinal" | jq -c '{shapeHash, keep, tail}')
[ "$(printf '%s\n' "$requests" | wc -l)" -eq "$ordinal" ] || { echo "fewer than $ordinal requests after line $from_line" >&2; exit 1; }
shape_hash=$(printf '%s\n' "$requests" | tail -n 1 | jq -r .shapeHash)

rows=$(jq -n -r --arg show "$show" \
  --slurpfile reqs <(printf '%s\n' "$requests") \
  --slurpfile blobs <(grep '"type":"api-request-blob"' "$transcript") \
  --slurpfile shapes <(grep '"type":"api-request-shape"' "$transcript" | grep -F "\"shapeHash\":\"$shape_hash\"" | head -n 1) \
  -f "$here/classify.jq")
if [ -n "$show" ]; then
  printf '%s\n' "$rows"
  exit 0
fi

# The reply to the measured request is the first assistant record after it; its usage counts every input token of the request.
request_line=$(tail -n +"$from_line" "$transcript" | grep -n '"type":"api-request"' | sed -n "${ordinal}p" | cut -d: -f1)
input_tokens=$(tail -n +"$((from_line + request_line))" "$transcript" | grep -m 1 '"type":"assistant"' \
  | jq '.message.usage | .input_tokens + .cache_creation_input_tokens + .cache_read_input_tokens')

printf '%s\n' "$rows" | awk -F'\t' -v tokens="$input_tokens" '
  { chars[$1] += $2; total += $2 }
  END {
    printf "input tokens\t%d\ncharacters\t%d\ncharacters per token\t%.2f\n\n", tokens, total, total / tokens
    for (source in chars) printf "%s\t%d\t%d\n", source, chars[source], chars[source] * tokens / total | "sort -t\"\t\" -k2,2nr"
  }'
