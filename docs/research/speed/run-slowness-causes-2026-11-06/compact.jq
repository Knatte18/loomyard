# compact.jq: reads one Claude Code session transcript line by line (jq -c) and prints one small record per entry that bears on time.
# A record's k names its kind: name (the session's strand name), req (an API request leaving), a (a block of an assistant message),
# r (tool results coming back), p (a prompt reaching the session), h (a hook that ran) and d (the client's own turn length).
# A prompt is one of launch (the skill-load line a session starts with), task (the pointer to its prompt file), gate (gate findings typed back),
# notice (a notice the client gives, such as a finished fork or creel mail) and other.
# Times are milliseconds since the epoch.

def ms: if . == null then null else ((.[0:19] + "Z") | fromdateiso8601) * 1000 + ((.[20:23] | tonumber?) // 0) end;
def brief:
  (.input // {}) as $i
  | if .name == "Bash" then ($i.command // "")
    elif .name == "Agent" or .name == "Task" then "\($i.subagent_type // "")/\($i.model // "") \($i.description // "")"
    elif .name == "Skill" then ($i.skill // "")
    elif .name == "Grep" or .name == "Glob" then ($i.pattern // "")
    else ($i.file_path // $i.path // "") end
  | gsub("[\\t\\n\\r]+"; " ") | .[0:160];

if .type == "agent-name" then {k: "name", name: .agentName}
elif .type == "api-request" then {k: "req", id, t: (.timestamp | ms)}
elif .type == "assistant" then
  (.message.usage // {}) as $u
  | {k: "a", t: (.timestamp | ms), mid: .message.id, ref: .requestRef, model: .message.model, effort: (.effort // ""),
     u: [($u.input_tokens // 0), ($u.cache_read_input_tokens // 0), ($u.cache_creation_input_tokens // 0), ($u.output_tokens // 0), ($u.output_tokens_details.thinking_tokens // 0)],
     stop: (.message.stop_reason // ""),
     tools: [.message.content[]? | select(.type == "tool_use") | [.id, .name, brief]]}
elif .type == "user" then
  (.message.content) as $c
  | if ($c | type) == "array" and ($c | any(.type == "tool_result")) then
      {k: "r", t: (.timestamp | ms), ids: [$c[] | select(.type == "tool_result") | .tool_use_id], chars: ([$c[] | select(.type == "tool_result") | .content | tostring | length] | add)}
    elif .isMeta == true then empty
    else
      ($c | if type == "array" then map(.text? // "") | join(" ") else tostring end) as $text
      | {k: "p", t: (.timestamp | ms), src: (.promptSource // ""), chars: ($text | length),
         prompt: (if ($text | startswith("Load these skills")) then "launch"
                  elif ($text | startswith("Read /")) then "task"
                  elif ($text | startswith("Gate findings")) then "gate"
                  elif .promptSource == "system" or ($text | startswith("<task-notification")) then "notice"
                  else "other" end)} end
elif .type == "attachment" and .attachment.hookEvent != null then {k: "h", t: (.timestamp | ms), hook: .attachment.hookEvent, dur: (.attachment.durationMs // 0)}
elif .type == "system" and .subtype == "turn_duration" then {k: "d", t: (.timestamp | ms), dur: .durationMs}
else empty end
