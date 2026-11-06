#!/usr/bin/env bash
# Usage: analyze.sh [dir] > tables.md
# Prints every table of the report as markdown from the tables in dir (default: this directory):
# run-tables.awk covers every landed run, and turn-tables.awk the runs of pick.tsv whose transcripts turns.sh read.
# The eras are cut by commit times, in UTC: f08df88b (the first agent-turns change), 6c5727df (one test binary per package),
# f5724733 (the origin check before every status push) and 48f17897 (a push's transfer outside the remote-refs lock).
# Needs bash, awk and git; reads only. PROJECTS names Claude Code's projects directory (default ~/.claude/projects).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
dir=${1:-$here}
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
projects=${PROJECTS:-$HOME/.claude/projects}
encoded=$(printf '%s' "$(dirname "$prime")" | tr -c 'A-Za-z0-9' '-')

# utc <commit>: the commit's committer time in UTC, as the timing report writes a start.
utc() { TZ=UTC git -C "$prime" log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ "$1"; }

have=$(for d in "$projects/$encoded"-*; do [ -d "$d" ] && printf '%s\n' "${d#"$projects/$encoded"-}"; done)

awk -f "$here/lib.awk" -f "$here/run-tables.awk" \
  -v cut_before="$(utc f08df88b)" -v binaries="$(utc 6c5727df)" -v check="$(utc f5724733)" -v unlocked="$(utc 48f17897)" -v here="$have" \
  "$dir/runs.tsv" "$dir/rows.tsv" "$dir/gate-runs.tsv" "$dir/reviews.tsv" "$dir/repo-size.tsv"
echo
awk -f "$here/lib.awk" -f "$here/turn-tables.awk" "$dir/pick.tsv" "$dir/timeline.tsv" "$dir/sessions.tsv" "$dir/turns.tsv" "$dir/tools.tsv"
