# session.jq: reads the compact records of one transcript as one array (jq -s) and prints tab-separated lines, the first field naming the table:
# S  slug session kind role first_ms last_ms
# T  slug session kind role message model effort start_ms first_ms end_ms input cache_read cache_write output thinking tools stop
# U  slug session kind role tool_use name category start_ms end_ms result_chars
# P  slug session kind role at_ms source chars prompt
# A T line is one API request: it starts when the request leaves (the req record its blocks point at) and ends at its last block.
# A subagent transcript holds no req record, so its request starts at the entry before the message's first block.
# A U line is one tool call, from the block that asks for it to the result that answers it; a call with no result is left out.
# $slug, $session, $kind (main or sub) and $role come from the caller; a main session's role is its strand name's last part where it has one.

def category($name; $brief):
  if $name == "Bash" then
    if ($brief | test("lym shuttle await")) then "agent"
    elif ($brief | test("lym gate (test|card-verify)|(^|[;&| ])go (test|build|vet)( |$)")) then "gate"
    elif ($brief | test("(^|[;&|] *|env -C [^ ]+ +)lym ")) then "lym"
    elif ($brief | test("^(cd [^ ]+ *(&&|;) *)?git ")) then "git"
    elif ($brief | test("^(cd [^ ]+ *(&&|;) *)?(grep|rg|find|ls|cat|sed -n|head|tail|wc|awk|jq)( |$)")) then "shell-read"
    else "shell-other" end
  elif $name == "Read" then "read"
  elif $name == "Grep" or $name == "Glob" then "search"
  elif $name == "Edit" or $name == "Write" or $name == "MultiEdit" or $name == "NotebookEdit" then "edit"
  elif $name == "Agent" or $name == "Task" then "agent"
  elif $name == "Skill" then "skill"
  else "other" end;

. as $recs
| ([$recs[] | select(.k == "name") | .name] | last) as $name
| (if $kind == "main" and $name != null then ($name | split(":") | last | sub("-[0-9]+$"; "")) else $role end) as $r
| (reduce ($recs[] | select(.k == "req")) as $q ({}; .[$q.id] = $q.t)) as $req
| (reduce ($recs[] | select(.k == "r")) as $res ({}; reduce $res.ids[] as $id (.; .[$id] = [$res.t, $res.chars]))) as $result
| (reduce ($recs[] | select(.k == "a" or .k == "r" or .k == "p")) as $e ({prev: null, last_mid: null, start: {}};
    if $e.k == "a" then
      (if .start[$e.mid] == null then .start[$e.mid] = (.prev // $e.t) else . end) | .prev = $e.t
    else .prev = $e.t end) | .start) as $fallback
| ([$recs[] | select(.t != null) | .t]) as $times
| (if ($times | length) > 0 then ["S", $slug, $session, $kind, $r, ($times | min), ($times | max)] else empty end),
  ([$recs[] | select(.k == "a")] | group_by(.mid)[]
    | . as $blocks
    | $blocks[0] as $b
    | ($req[$b.ref // ""] // $fallback[$b.mid]) as $start
    | ["T", $slug, $session, $kind, $r, $b.mid, ($b.model // ""), $b.effort, $start, ($blocks | map(.t) | min), ($blocks | map(.t) | max),
       ($blocks | map(.u[0]) | max), ($blocks | map(.u[1]) | max), ($blocks | map(.u[2]) | max), ($blocks | map(.u[3]) | max), ($blocks | map(.u[4]) | max),
       ($blocks | map(.tools | length) | add), ($blocks | map(.stop) | map(select(. != "")) | last // "")]),
  ($recs[] | select(.k == "a") | . as $a | .tools[] | . as [$id, $tool, $brief]
    | select($result[$id] != null)
    | ["U", $slug, $session, $kind, $r, $id, $tool, category($tool; $brief), $a.t, $result[$id][0], $result[$id][1]]),
  ($recs[] | select(.k == "p") | ["P", $slug, $session, $kind, $r, .t, .src, .chars, .prompt])
| @tsv
