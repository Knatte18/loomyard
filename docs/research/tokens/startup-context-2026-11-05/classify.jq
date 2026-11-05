# classify.jq sorts every character of one rebuilt Claude Code API request into the source that put it there.
# Inputs: $reqs, the session's api-request records up to and including the measured one;
# $blobs, every api-request-blob record of the transcript; $shapes, the api-request-shape record the measured request names.
# With $show empty it prints one "source<TAB>characters" row per line of text;
# with $show naming a source it prints that source's lines instead, to check the sorting.

# install_path names a CLAUDE.md-style file by the role it plays rather than its machine path.
def install_path:
  if test("/\\.claude/CLAUDE\\.md$") then "user's global ~/.claude/CLAUDE.md"
  elif test("/memory/MEMORY\\.md$") then "auto-memory MEMORY.md index"
  elif test("CLAUDE\\.local\\.md$") then "lym: CLAUDE.local.md (heddle)"
  elif test("CLAUDE\\.md$") then "repository CLAUDE.md"
  else . end;

# system_line_source sorts the line in .line of the system prompt's last block, given the source of the line before it in .state.
# It returns the source the next line inherits and the source of this line.
def system_line_source:
  .state as $state
  | .line
  | (if test("^(AskUserQuestion is unavailable|python and python3 are refused)") then "lym: append: deny notices and creel trust"
   elif test("^# Orch rules") then "lym: append: hub orch rules"
   elif ($state | startswith("lym")) then $state
   elif test("^# Memory$") then "cc: system prompt: auto-memory instructions"
   elif test("^# ") then "cc: system prompt: " + (.[2:] | ascii_downcase)
   elif test("^(Do build original games|EndConversation|WebSearch takes)") then "cc: system prompt: closing notes"
   else $state end)
  | [., .];

# message_line_source sorts the line in .line of message text, given the source of the line before it in .state.
# It returns the source the next line inherits and the source of this line, which differ only for a connector's deferred tool name.
def message_line_source:
  .state as $state
  | .line as $line
  | $line
  | (if test("^Codebase and user instructions are shown below") then "instructions: preamble"
     elif test("^Contents of [^ ]+") then "instructions: " + (capture("^Contents of (?<p>[^ ]+)").p | install_path)
     elif test("^As you answer the user's questions") then "cc: session context (email, git status)"
     elif test("^SessionStart:[a-z]+ hook") then "hook: SessionStart (scribe)"
     elif test("^[A-Za-z]+ hook (success|additional context)") then "hook: other"
     elif test("^# Environment$") then "cc: environment"
     elif test("^The following deferred tools are now available") then "cc: deferred tool names"
     elif test("^Available agent types") then "cc: agent types"
     elif test("^The following skills are available") then "skills listing: header"
     elif ($state | startswith("skills listing")) and test("^- [^: ]+:[^: ]+:") then "skills listing: plugin " + capture("^- (?<p>[^: ]+):").p
     elif ($state | startswith("skills listing")) and test("^- [^: ]+:") then "skills listing: built-in"
     elif test("^While bypass permissions mode") then "cc: bypass-mode note"
     elif test("^Today's date is") then "cc: date"
     elif test("^# MCP Server Instructions") then "connectors: MCP instructions"
     elif test("^The following tools just became available") then "connectors: loaded tool schemas"
     elif test("^Base directory for this skill: ") then "skill body: " + (capture("skills/(?<s>[^/ ]+)$").s // "unknown")
     elif test("^This session is being continued from a previous conversation") then "compact: summary"
     elif test("^The following skills were invoked EARLIER") then "compact: re-injected skill bodies"
     elif test("^(Result of calling|Called) the Read tool") then "compact: re-read files"
     elif test("^Read .*/prompt\\.md in full first") then "lym: role prompt pointer"
     elif test("^Load these skills now") then "lym: skill preload request"
     elif test("^\\[creel\\]") then "creel mail"
     elif test("^<command-name>/color</command-name>") then "lym: pane color command"
     else $state end)
  | if . == "cc: deferred tool names" and ($line | test("^mcp__")) then [., "connectors: deferred tool names"]
    else [., .] end;

# sorted_lines pairs each line of a text with its source, starting from the given default source and sorting with f.
def sorted_lines($default; f):
  split("\n")
  | reduce .[] as $line ({state: $default, rows: []};
      ({line: $line, state: .state} | f) as $sorted
      | .state = $sorted[0]
      | .rows += [[$sorted[1], $line]])
  | .rows[];

def system_rows:
  $shapes[0].shape.system | to_entries[]
  | if .key < 2 then ["cc: system prompt: intro", .value.text]
    else (.value.text | sorted_lines("cc: system prompt: intro"; system_line_source)) end;

def tool_rows:
  $shapes[0].shape.tools[]
  | [if ((.name // "") | startswith("mcp__")) then "connectors: tool schemas" else "cc: tool schemas" end, tostring];

def block_rows($role):
  if .type == "text" then (.text | sorted_lines("message: " + $role + " text"; message_line_source))
  elif .type == "tool_use" then ["assistant: tool calls", (.input | tostring)]
  elif .type == "thinking" then ["assistant: thinking", (.thinking // "")]
  elif .type == "tool_result" then
    ["tool results", (if (.content | type) == "string" then .content else (.content | map(.text // tostring) | join("\n")) end)]
  elif .type == "tool_addition" then
    [if (.tool.definition.name // "" | startswith("mcp__")) then "connectors: loaded tool schemas" else "cc: loaded tool schemas" end, tostring]
  else ["message: " + .type, tostring] end;

def message_rows:
  (reduce $blobs[] as $b ({}; .[$b.hash] = $b.message)) as $map
  | (reduce $reqs[] as $r ([]; .[0:$r.keep] + $r.tail))[]
  | $map[.] as $message
  # A few assistant replies carry no blob record; they stay in the table as an empty row so a reader sees one is missing.
  | if $message == null then ["unrecorded message", ""]
    elif ($message.content | type) == "string" then ({type: "text", text: $message.content} | block_rows($message.role))
    else ($message.content[] | block_rows($message.role)) end;

if $show == "" then (system_rows, tool_rows, message_rows) | "\(.[0])\t\((.[1] | length) + 1)"
else (system_rows, tool_rows, message_rows) | select(.[0] == $show) | .[1] end
