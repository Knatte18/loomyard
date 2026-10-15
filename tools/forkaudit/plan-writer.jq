# plan-writer.jq emits one TSV row for a top-level session transcript: its plan card writes and its lookup tool use.
# Use: jq -n -R -r --arg session <session id> --arg plan <plan directory> -f plan-writer.jq <session transcript>
# Columns: session id, plan card writes, quarry calls, greps, reads.
# Greps and reads count both the Grep and Read tools and the matching shell commands.
([ inputs | fromjson? | select(type == "object" and .type == "assistant") | .message.content[]? | select(.type == "tool_use") ]) as $tools
| ([ $tools[] | select(.name == "Bash") | (.input.command // "") | [splits("&&|\\|\\||;|\\n")] | .[] | sub("^\\s+"; "") ]) as $segments
| def shell($pattern): [ $segments[] | select(test($pattern)) ] | length;
  [ $session,
    ([ $tools[] | select(.name == "Write" and ((.input.file_path // "") | test("(^|/)" + $plan + "[0-9][0-9]-.*\\.md$"))) ] | length),
    shell("^lyx (quarry|glyph)"),
    ([ $tools[] | select(.name == "Grep") ] | length) + shell("^(git )?(grep|rg)( |$)"),
    ([ $tools[] | select(.name == "Read") ] | length) + shell("^(cat|sed -n|head|tail|nl|awk) ")
  ] | @tsv
