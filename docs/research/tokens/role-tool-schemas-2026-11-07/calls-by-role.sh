#!/usr/bin/env bash
# calls-by-role.sh counts, per lym role, which tools its sessions carried and called, over the transcripts of one projects directory.
# A session's role is the last segment of its agent name, `<prefix>:<slug>:<role>`; a name with fewer segments is its own role.
# A tool is carried when the session's first api-request-shape record lists its schema.
# A tool call is counted once per tool-use id, so a record the transcript repeats does not count twice.
# Subagent transcripts, which sit below a session's own directory, are not read: they hold the subagents' calls, not the role's.
#
# Usage: calls-by-role.sh [PROJECTS_DIR]
#   PROJECTS_DIR  defaults to ~/.claude/projects
#
# Output, tab separated, one row per role and tool: role, tool, sessions that carried it, calls, sessions that called it.
# A tool a session called without carrying its schema is a deferred tool, loaded through ToolSearch.
# A row per role with tool "(sessions)" gives the role's session count in the third column.
set -eu

projects=${1:-$HOME/.claude/projects}

for transcript in "$projects"/*/*.jsonl; do
  name=$(grep -m 1 -o '"type":"agent-name","agentName":"[^"]*"' "$transcript" | sed 's/.*"agentName":"//; s/"$//') || true
  [ -n "$name" ] || continue
  role=${name##*:}
  printf 'S\t%s\n' "$role"
  grep -m 1 '"type":"api-request-shape"' "$transcript" | jq -r '.shape.tools[].name' 2>/dev/null \
    | awk -v role="$role" '{ printf "C\t%s\t%s\n", role, $1 }' || true
  grep -o '"type":"tool_use","id":"[^"]*","name":"[^"]*"' "$transcript" | sort -u \
    | sed 's/.*"name":"//; s/"$//' | sort | uniq -c \
    | awk -v role="$role" '{ printf "K\t%s\t%s\t%d\n", role, $2, $1 }' || true
done | awk -F'\t' '
  $1 == "S" { sessions[$2]++; next }
  $1 == "C" { carried[$2 FS $3]++; seen[$2 FS $3] = 1; next }
  $1 == "K" { calls[$2 FS $3] += $4; callers[$2 FS $3]++; seen[$2 FS $3] = 1 }
  END {
    for (key in seen) print key "\t" carried[key] + 0 "\t" calls[key] + 0 "\t" callers[key] + 0
    for (role in sessions) print role "\t(sessions)\t" sessions[role] "\t0\t0"
  }' | sort -t$'\t' -k1,1 -k4,4nr
