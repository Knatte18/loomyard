#!/usr/bin/env bash
# Cross-check: run the previous audit's extract2.jq (copied unchanged to extract2.jq here) over the same Webster transcripts.
set -u
here="$(cd "$(dirname "$0")" && pwd)"
cd "$HOME/.claude/projects" || exit 1
: > "$here/raw-old.jsonl"
while IFS= read -r f; do
  jq -s -c -f "$here/extract2.jq" "$f" >> "$here/raw-old.jsonl"
done < "$here/webster-files.txt"
wc -l "$here/raw-old.jsonl"
