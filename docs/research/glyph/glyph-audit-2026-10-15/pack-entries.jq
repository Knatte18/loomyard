# Emits one TSV row per pack entry of a fork's pack file (the Read result of *-pack.md).
# Use: jq -s -r -f pack-entries.jq <transcript>   Columns: agent, card, kind(span|unres), glyph, file, a, b, sig_or_reason
def rtext: if type=="array" then map(.text // "")|join("") elif type=="string" then . else "" end;
. as $all
| (input_filename|split("/")|last) as $agent
| ([ $all[] | select(.type=="assistant") | .message.content[]? | select(.type=="tool_use" and .name=="Read" and ((.input.file_path//"")|test("-pack\\.md$"))) ] | .[0].id) as $pid
| ([ $all[] | select(.type=="user") | .message.content | if type=="array" then .[] | select(.type=="tool_result" and .tool_use_id==$pid) | (.content|rtext) else empty end ] | .[0] // "") as $txt
| reduce ($txt | split("\n")[] | sub("^\\s*[0-9]+\t";"")) as $l ({card:"",u:false,g:"",rows:[]};
    if ($l|test("^### `")) then .card=($l|capture("^### `(?<c>[^`]+)`").c) | .u=false
    elif ($l|test("^#### Unresolved")) then .u=true
    elif ($l|test("^- `")) then
      ($l|capture("^- `(?<g>[^`]+)`(?: — (?<r>.*))?")) as $c
      | if .u then .rows += [[$agent,.card,"unres",$c.g,"","","",($c.r//"")]] else .g=$c.g end
    elif ($l|test("^  - `[^`:]+:[0-9]+(-[0-9]+)?` — ")) then
      ($l|capture("^  - `(?<f>[^`:]+):(?<a>[0-9]+)(-(?<b>[0-9]+))?` — (?<s>.*)")) as $c
      | .rows += [[$agent,.card,"span",.g,$c.f,$c.a,($c.b//$c.a),$c.s]]
    else . end)
| .rows[] | @tsv
