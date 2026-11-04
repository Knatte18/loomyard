#!/usr/bin/env bash
# Usage: events.sh <out-dir>
# Reads the Claude Code transcripts of the slugs in slugs.txt (main sessions and fork sub-agents) under
# ~/.claude/projects and writes <out-dir>/events.tsv, one row per hook firing, refused call or gate result:
# slug session kind role type turn a b c
# turn numbers a session's assistant messages as extract-turns.sh of the turn audit does, so rows join turns.tsv on slug, session and turn.
#   type nudge:   a = the breach (lookup, slice, edit), b = the call count of the next message, c = the hook's tool
#   type refusal: a = the refused rule (shell-edit, python, rm, go-run, other), b = the tool, c empty
#   type gate:    a = the gate (card, batch, other), b = the result (fail, green, pending), c empty
# A gate result is fail when the tool result opens "Exit code" or "exit" and a nonzero code, the two shapes the harness and lym's wait mode give, and green otherwise.
# A call that moved to the background or answered done:false takes its exit code from the task notification that reports it, 0 green and any other fail;
# with no such notification, or no result at all in the transcript, it is pending.
# Sessions that started before CUTOFF (default 2026-11-02) are left out.
set -euo pipefail
out=$1
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cutoff=${CUTOFF:-2026-11-02}
projects=${PROJECTS:-$HOME/.claude/projects}
mkdir -p "$out"
printf 'slug\tsession\tkind\trole\ttype\tturn\ta\tb\tc\n' > "$out/events.tsv"

process() { # file slug session kind role
  local file=$1 slug=$2 session=$3 kind=$4 role=$5
  jq -rs -f "$here/events.jq" "$file" |
    awk -F'\t' -v OFS='\t' -v s="$slug" -v se="$session" -v k="$kind" -v r="$role" '{ print s, se, k, r, $0 }' >> "$out/events.tsv"
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
