#!/usr/bin/env bash
# Usage: outcomes.sh <slug>...
# Prints per run, tab-separated:
#   turn  <slug> <session> <time> <test files edited before the next typed message> <test functions those edits add>
#   mut   <slug> <mutants run> <killed> <survived> <uncovered blocks>   one per distinct mutation summary line
#   red   <slug> <contained> <unique> <not judged>                     one per distinct redundancy summary line
#   run   <slug> <reach turns> <verify logs>
# A reach turn is the reach note closed by "Add the missing tests and commit them".
# Report lines count only when they name the run's own worktree, since a verify log also holds the output of tests.
# A new test function is one a Write or Edit puts in that the text it replaced did not hold.
# Needs bash, jq and an awk; reads only.
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
prime=${PRIME:-$(git -C "$here" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
hub=${HUB:-$(dirname "$prime")}
archive=${ARCHIVE:-$hub/_bolt/.lym/log-archive}
projects=${PROJECTS:-$HOME/.claude/projects}
for slug in "$@"; do
  sessions="$projects/$(printf '%s' "$hub" | tr -c 'A-Za-z0-9' '-')-$slug"
  turns=0
  for f in "$sessions"/*.jsonl; do
    [ -e "$f" ] || continue
    grep -q 'Add the missing tests and commit them' "$f" || continue
    id=$(basename "$f" .jsonl | cut -c1-8)
    out=$(jq -rc '
      def typed: .type == "user" and (.message.content | type == "string");
      def results: .message.content | arrays | .[] | select(.type == "tool_result") | .content
        | if type == "string" then . else ([.[]? | .text // ""] | join("\n")) end;
      def funcs: [scan("(?m)^func (Test[A-Za-z0-9_]*)") | .[0]];
      # The turn reaches a session as a read of the gate findings file, or as a typed message.
      def reachturn: .type == "user" and ([(if typed then .message.content else results end)
        | select(test("^(1\\t)?(## Gate verify\\n(2\\t)?\\n(3\\t)?)?gate test: reach: ") and test("Add the missing tests and commit them"))]
        | length > 0);
      if reachturn then "TURN\t\(.timestamp)"
      elif typed then "END"
      elif .type == "assistant" then (.message.content[]? | select(.type == "tool_use" and (.name == "Edit" or .name == "Write"))
        | select(.input.file_path | test("_test\\.go$"))
        | "EDIT\t\(.input.file_path)\t\(((.input.new_string // .input.content // "") | funcs) - ((.input.old_string // "") | funcs) | length)")
      else empty end' "$f" 2>/dev/null |
      awk -F'\t' -v slug="$slug" -v id="$id" '
        function flush() { if (on) { n = 0; for (k in files) n++; print "turn\t" slug "\t" id "\t" at "\t" n "\t" added; split("", files); added = 0; on = 0 } }
        $1 == "TURN" { flush(); on = 1; at = $2 }
        $1 == "END" { flush() }
        $1 == "EDIT" && on { files[$2] = 1; added += $3 }
        END { flush() }')
    [ -n "$out" ] && printf '%s\n' "$out" && turns=$((turns + $(printf '%s\n' "$out" | wc -l)))
  done
  logs=$(ls "$archive/$slug"/*/verify/verify-*.log 2>/dev/null | wc -l)
  if [ "$logs" -gt 0 ]; then
    cat "$archive/$slug"/*/verify/verify-*.log |
      grep -E "^gate test: (mutation|redundancy) report: .*$hub/$slug/" |
      awk -v slug="$slug" '
        /mutation report: [0-9]+ mutants run/ {
          match($0, /[0-9]+ mutants run/); m = $0; sub(/.*mutation report: /, "", m)
          split(m, w, /[ ,]+/); line = "mut\t" slug "\t" w[1] "\t" w[4] "\t" w[6] "\t" w[10] }
        /redundancy report: [0-9]+ contained/ {
          m = $0; sub(/.*redundancy report: /, "", m); split(m, w, /[ ,]+/); line = "red\t" slug "\t" w[1] "\t" w[3] "\t" w[5] }
        line != "" && !seen[line]++ { print line }
        { line = "" }'
  fi
  printf 'run\t%s\t%s\t%s\n' "$slug" "$turns" "$logs"
done
