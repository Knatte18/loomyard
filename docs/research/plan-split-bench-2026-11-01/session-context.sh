#!/usr/bin/env bash
# Per-session turns and context of the plan-split bench runs, from Claude's session transcripts.
# Bash and jq only.
#
# Usage:
#   session-context.sh <projects-dir> <hub-dir> <run>...
#
# <projects-dir> is Claude Code's projects directory (usually $HOME/.claude/projects),
# <hub-dir> the hub holding the run worktrees, and each <run> a worktree's directory name.
# A project directory is named after the worktree path with every character outside [A-Za-z0-9] replaced by "-".
#
# Output: one tab-separated line per transcript, a main session or a fork under it, on stdout:
#   run, kind (main or fork), title, turns, mean context, peak context, first timestamp, last timestamp
# A turn is one assistant message, counted once however many records carry it.
# A message's context is its input, cache-write and cache-read tokens.
# A fork has the title of its main session with "+fork" appended.
set -euo pipefail

if [ "$#" -lt 3 ]; then
	echo "usage: session-context.sh <projects-dir> <hub-dir> <run>..." >&2
	exit 2
fi
projects_dir=$1
hub_dir=$2
shift 2

summarize() {
	jq -s -r --arg run "$1" --arg kind "$2" --arg title "$3" '
		[ .[] | select(.type == "assistant" and .message.usage != null)
		  | { id: .message.id, ts: .timestamp,
		      ctx: (.message.usage.input_tokens + .message.usage.cache_creation_input_tokens + .message.usage.cache_read_input_tokens) } ]
		| group_by(.id) | map(.[-1]) | sort_by(.ts)
		| select(length > 0)
		| [ $run, $kind, $title, length,
		    ((map(.ctx) | add) / length | floor),
		    (map(.ctx) | max), .[0].ts, .[-1].ts ]
		| @tsv' "$4"
}

for run in "$@"; do
	encoded=$(printf '%s' "$hub_dir/$run" | sed 's/[^A-Za-z0-9]/-/g')
	project="$projects_dir/$encoded"
	[ -d "$project" ] || { echo "no project directory for $run: $project" >&2; continue; }
	for main in "$project"/*.jsonl; do
		[ -e "$main" ] || continue
		title=$(jq -r 'select(.type == "custom-title") | .customTitle' "$main" | tail -1)
		title=${title:-untitled}
		summarize "$run" main "$title" "$main"
		session_dir=${main%.jsonl}
		for fork in "$session_dir"/subagents/agent-*.jsonl; do
			[ -e "$fork" ] || continue
			summarize "$run" fork "$title+fork" "$fork"
		done
	done
done
