#!/usr/bin/env bash
# Usage: turns.sh <slug>
# Prints every reach turn the run's sessions received: the session, the time, and the findings text,
# found by the turn's way forward "Add the missing tests and commit them".
# Needs bash and jq; reads only.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
for f in "$(sessions "$1")"/*.jsonl; do
  jq -r '
    select(.type=="user") | .timestamp as $t
    | (.message.content | if type=="string" then [.] else [.[]? | (.text // (.content
        | if type=="array" then (map(.text // "") | join("\n")) else (. // "") end))] end)[]
    | select(test("Add the missing tests and commit them"))
    | "\($t)\n\(.[0:2500])\n----"' "$f" 2>/dev/null | sed "s|^|$(basename "$f" .jsonl | cut -c1-8) |"
done
