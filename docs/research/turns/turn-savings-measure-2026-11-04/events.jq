# Reads one session transcript, slurped, and prints tab-separated event rows: type turn a b c (see events.sh).

def restext: (.content | if type == "array" then map(.text? // "") | join(" ") else (. // "") | tostring end);
def refusal_rule($t):
  if ($t | test("never run Python")) then "python"
  elif ($t | test("sed -i, perl -i, and a heredoc")) then "shell-edit"
  elif ($t | test("rm on a computed path")) then "rm"
  elif ($t | test("raw module-wide|go test run is refused|go run")) then "go-run"
  else "other" end;
def nudge_kind($t):
  if ($t | test("each made one lookup")) then "lookup"
  elif ($t | test("read a slice of")) then "slice"
  elif ($t | test("only edits")) then "edit"
  else "other" end;

. as $all
| ([$all[] | select(.type == "assistant" and .message.id != null)]
   | group_by(.message.id) | map(sort_by(.timestamp)) | sort_by(.[0].timestamp)
   | map([.[] | .message.content[]? | select(.type == "tool_use")] | unique_by(.id))) as $turns
| ([range(0; $turns | length) as $n | $turns[$n][] | {key: .id, value: {turn: ($n + 1), name: .name, command: (.input.command // "")}}] | from_entries) as $call
| ([$all[] | select(.type == "user") | (.message.content | arrays | .[]) | select(.type == "tool_result")
    | {key: .tool_use_id, value: {text: restext, error: (.is_error // false)}}] | from_entries) as $res
# exit codes of calls that ran in the background, from the task notifications that report them, keyed by the call's tool use id
| ([$all[] | select(.type == "user" or .type == "queue-operation") | .. | strings | select(contains("<task-notification>"))
    | capture("<tool-use-id>(?<id>[^<]+)</tool-use-id>.*?exit code (?<code>[0-9]+)\\)"; "s") | {key: .id, value: (.code | tonumber)}] | from_entries) as $background
| (
  # hook firings: the PostToolUse attachment that carries the hook's stdout
  ($all[] | select(.type == "attachment" and (.attachment.stdout? // "" | contains("[lym batching]")))
   | .attachment as $a | $call[$a.toolUseID] as $c | select($c != null)
   | "nudge\t\($c.turn)\t\(nudge_kind($a.stdout))\t\(($turns[$c.turn] // []) | length)\t\($c.name)"),
  # refused calls: a tool result that opens with a PreToolUse hook error
  ($call | to_entries[] | .key as $id | .value as $c | $res[$id] as $r
   | select($r != null and ($r.text | test("^PreToolUse:[A-Za-z]+ hook error")))
   | "refusal\t\($c.turn)\t\(refusal_rule($r.text))\t\($c.name)\t"),
  # lym gate test calls and how each ended
  ($call | to_entries[] | .key as $id | .value as $c
   | select($c.name == "Bash" and ($c.command | test("lym gate test")))
   | $res[$id] as $r
   | ((if ($c.command | test("--gate card")) then "card" elif ($c.command | test("--gate batch")) then "batch" else "other" end)) as $g
   | (if $r == null or ($r.text | test("^Command was moved to the background|\"done\":false|the run is still going")) then
        (if $background[$id] == null then "pending" elif $background[$id] == 0 then "green" else "fail" end)
      elif ($r.text | test("^([Ee]xit code|exit) [1-9]")) then "fail"
      else "green" end) as $o
   | "gate\t\($c.turn)\t\($g)\t\($o)\t")
)
