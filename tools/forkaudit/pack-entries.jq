# pack-entries.jq emits one TSV row per pack entry of a Webster fork.
# The entries come from the `<batch>-pack.md` file the fork read, or from the prompt's kick-start pack section when it read none.
# Use: jq -n -R -r --arg rel <transcript path relative to the projects directory> -f pack-entries.jq <transcript>
# Columns: rel, card, kind (span or unres), glyph, file, first line, last line, signature or reason.
def rtext: if type=="array" then map(.text // "")|join("") elif type=="string" then . else "" end;

([inputs | fromjson? | select(type == "object")]) as $all
| ([ $all[] | select(.type == "assistant") | .message.content[]? | select(.type == "tool_use" and .name != "Skill" and .name != "Agent") ]) as $tools
| (reduce ($all[] | select(.type == "user") | .message.content | if type == "array" then .[] | select(.type == "tool_result") else empty end) as $r
    ({}; .[$r.tool_use_id] = ($r.content | rtext))) as $results
| ($tools | map(select(.name == "Read" and ((.input.file_path // "") | test("-pack\\.md$")))) | .[0].id) as $pack_id
| ($tools[0] | if . != null and .name == "Read" and ((.input.file_path // "") | test("/webster/prompts/|prompt\\.md$")) and ((.input.file_path // "") | test("-pack\\.md$") | not)
    then $results[.id] else null end // "") as $prompt
| (if $pack_id != null then ($results[$pack_id] // "")
   elif ($prompt | test("## Kick-start pack")) then ($prompt | split("## Kick-start pack")[1])
   else "" end) as $text
| reduce ($text | split("\n")[] | sub("^\\s*[0-9]+\t"; "")) as $l ({card: "", u: false, g: "", rows: []};
    if ($l | test("^### `")) then .card = ($l | capture("^### `(?<c>[^`]+)`").c) | .u = false
    elif ($l | test("^#### Unresolved")) then .u = true
    elif ($l | test("^- `")) then
      ($l | capture("^- `(?<g>[^`]+)`(?: — (?<r>.*))?")) as $c
      | if .u then .rows += [[$rel, .card, "unres", $c.g, "", "", "", ($c.r // "")]] else .g = $c.g end
    elif ($l | test("^  - `[^`:]+:[0-9]+(-[0-9]+)?` — ")) then
      ($l | capture("^  - `(?<f>[^`:]+):(?<a>[0-9]+)(-(?<b>[0-9]+))?` — (?<s>.*)")) as $c
      | .rows += [[$rel, .card, "span", .g, $c.f, $c.a, ($c.b // $c.a), $c.s]]
    else . end)
| .rows[] | @tsv
