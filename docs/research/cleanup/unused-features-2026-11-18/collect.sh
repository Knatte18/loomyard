#!/bin/bash
# Counts, per lym verb, the transcript Bash commands that invoke it, and prints "count verb" sorted ascending.
# Usage: collect.sh <since-date, e.g. 2026-10-28> ; run with `lym` on PATH, from any folder.
set -eu
since="$1"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

walk() {
  local path="$1" subs
  subs=$(lym $path --json 2>/dev/null | jq -r '.commands // [] | .[].name')
  echo "$path"
  for s in $subs; do
    if [ "$s" = "help" ] || [ "$s" = "completion" ]; then continue; fi
    walk "$path $s"
  done
}

modules=$(lym help 2>&1 | sed -n 's/^Available modules: \(.*\)\.$/\1/p' | tr -d ',')
for m in $modules; do
  if [ "$m" = "help" ]; then continue; fi
  walk "$m"
done > "$work/verbs.txt"

find ~/.claude/projects -name '*.jsonl' -newermt "$since" -print0 \
  | xargs -0 -P8 grep -ohP '"command":"(?:[^"\\]|\\.)*"' 2>/dev/null \
  | grep -oP '(?<![\w/-])lym (?:[a-z][a-z-]*)(?: [a-z][a-z-]*){0,2}' > "$work/uses.txt"

while read -r verb; do
  echo "$(grep -cE "^lym $verb( |\$)" "$work/uses.txt") $verb"
done < "$work/verbs.txt" | sort -n
