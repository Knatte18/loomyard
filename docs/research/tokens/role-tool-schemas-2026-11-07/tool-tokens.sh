#!/usr/bin/env bash
# tool-tokens.sh prints the size of each tool schema in one API request of a Claude Code session transcript.
# It reads the tool array of the api-request-shape record the measured request names, and takes the request's
# characters-per-token ratio from measure.sh, so the tokens are on the same footing as measure.sh's table:
# the input tokens of the reply, split in proportion to characters.
#
# Usage: tool-tokens.sh TRANSCRIPT FROM_LINE REQUEST_ORDINAL
#   The arguments are measure.sh's: FROM_LINE is 1 for a fresh session, and REQUEST_ORDINAL counts from 1.
#
# Output, tab separated, one row per tool in array order: name, characters, tokens, then a total row.
# A deferred tool holds only its stub in the request, and its row shows that stub.
set -eu

transcript=$1
from_line=$2
ordinal=$3
here=$(cd "$(dirname "$0")" && pwd)
measure=$here/../startup-context-2026-11-05/measure.sh

requests=$(tail -n +"$from_line" "$transcript" | grep '"type":"api-request"' | head -n "$ordinal" | jq -c '{shapeHash}')
shape_hash=$(printf '%s\n' "$requests" | tail -n 1 | jq -r .shapeHash)
ratio=$("$measure" "$transcript" "$from_line" "$ordinal" | awk -F'\t' '
  $1 == "input tokens" { tokens = $2 }
  $1 == "characters" { characters = $2 }
  END { printf "%.6f", characters / tokens }')

grep '"type":"api-request-shape"' "$transcript" | grep -F "\"shapeHash\":\"$shape_hash\"" | head -n 1 \
  | jq -r '.shape.tools[] | "\(.name)\t\(tostring | length + 1)"' \
  | awk -F'\t' -v ratio="$ratio" '
    { printf "%s\t%d\t%d\n", $1, $2, $2 / ratio + 0.5; chars += $2; tokens += $2 / ratio }
    END { printf "total\t%d\t%d\n", chars, tokens + 0.5 }'
