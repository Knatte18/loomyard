# transcript.jq reads one Claude Code session transcript, slurped, and prints its retro rows, tab-separated:
#   S  title turns calls lookup_turns serial_saved refusals gate_runs gate_fails
#   R  turn tool refusal
# A turn is one assistant message id.
# A lookup turn makes exactly one call, and that call reads or searches.
# serial_saved counts the lookup turns that directly follow another lookup turn: each could have shared the earlier turn.
# An R row is a call a PreToolUse hook refused, with the first words of the refusal.
# A gate run is a `lym gate test` call.
# It failed when its result opens with a nonzero exit code,
# or when the task notification of a call moved to the background reports one.

# segwords splits a shell command into its segments,
# with quoted text, redirects of descriptors and leading assignments removed.
def segwords($cmd):
  [ $cmd
    | gsub("\"[^\"]*\""; "Q") | gsub("'[^']*'"; "Q")
    | gsub("[0-9]*>&[0-9]"; "") | gsub("[0-9]*>>?\\s*/dev/null"; "")
    | split("[;&|\\n]+"; "g")[]
    | gsub("^\\s+|\\s+$"; "")
    | gsub("^((for|while|if|then|do|else|elif)\\s+)+"; "")
    | gsub("^([A-Za-z_][A-Za-z0-9_]*=(\"[^\"]*\"|\\S*)\\s*)+"; "")
    | select(length > 0) ];

# lookup_bash reports whether a shell command only reads: every segment runs a read-only program and nothing is written.
def lookup_bash($cmd):
  ($cmd | gsub("[0-9]*>&[0-9]"; "") | gsub("[0-9]*>>?\\s*/dev/null"; "")) as $c
  | ($c | test("sed -i|perl -[0-9a-z]*i|\\btee\\b|<<|(^|\\s)>>?\\s*[^&\\s=]") | not)
    and (segwords($cmd) | length > 0)
    and (segwords($cmd) | all(
      test("^(cd|echo|printf|pwd|true|test|\\[|done|fi|basename|dirname|export|set)\\b")
      or test("^(cat|head|tail|sed|nl|wc|cut|sort|uniq|column|tr|diff|jq|stat|file|grep|rg|find|ls|tree|awk|xargs|readlink|realpath)\\b")
      or test("^git( -C \\S+)? (status|log|diff|show|rev-parse|branch|ls-files|ls-tree|blame|cat-file|merge-base)\\b")
      or test("^lym (quarry|board (get|list|find)|loom (peers|show)|[a-z]+ (status|list|show|get))\\b")));

# lookup reports whether a call reads or searches.
def lookup($call):
  if $call.name == "Read" or $call.name == "Grep" or $call.name == "Glob" then true
  elif $call.name == "Bash" then lookup_bash(($call.input.command // "") | tostring)
  else false end;

def restext: (.content | if type == "array" then map(.text? // "") | join(" ") else (. // "") | tostring end);

. as $all
| ([$all[] | select(.type == "custom-title") | .customTitle] | last // "") as $title
| ([$all[] | select(.type == "assistant" and .message.id != null)]
   | group_by(.message.id) | map(sort_by(.timestamp)) | sort_by(.[0].timestamp)
   | map([.[] | .message.content[]? | select(.type == "tool_use")] | unique_by(.id))) as $turns
| ([range(0; $turns | length) as $n | $turns[$n][] | {key: .id, value: {turn: ($n + 1), name: .name, command: ((.input.command // "") | tostring)}}] | from_entries) as $call
| ([$all[] | select(.type == "user") | (.message.content | arrays | .[]) | select(.type == "tool_result")
    | {key: .tool_use_id, value: restext}] | from_entries) as $res
# exit codes of calls that ran in the background, from the task notifications that report them,
# keyed by the call's tool use id
| ([$all[] | select(.type == "user" or .type == "queue-operation") | .. | strings | select(contains("<task-notification>"))
    | capture("<tool-use-id>(?<id>[^<]+)</tool-use-id>.*?exit code (?<code>[0-9]+)\\)"; "s") | {key: .id, value: (.code | tonumber)}] | from_entries) as $background
| [$turns[] | (length == 1 and lookup(.[0]))] as $lookups
| ([range(1; $lookups | length) | select($lookups[.] and $lookups[. - 1])] | length) as $serial
| [$call | to_entries[] | .key as $id | .value as $c | $res[$id] as $r
   | select($r != null and ($r | test("^PreToolUse:[A-Za-z]+ hook error")))
   | {turn: $c.turn, tool: $c.name,
      refusal: ($r | sub("^PreToolUse:[A-Za-z]+ hook error:?\\s*"; "") | gsub("[\\t\\n\\r]+"; " ") | .[0:100])}] as $refused
| [$call | to_entries[] | .key as $id | .value as $c
   | select($c.name == "Bash" and ($c.command | test("lym gate test")))
   | $res[$id] as $r
   | if $r == null or ($r | test("^Command was moved to the background|\"done\":false")) then ($background[$id] // 0) != 0
     else ($r | test("^([Ee]xit code|exit) [1-9]")) end] as $gates
| ( "S\t\($title)\t\($turns | length)\t\([$turns[] | length] | add // 0)\t\([$lookups[] | select(.)] | length)\t\($serial)\t\($refused | length)\t\($gates | length)\t\([$gates[] | select(.)] | length)",
    ($refused[] | "R\t\(.turn)\t\(.tool)\t\(.refusal)") )
