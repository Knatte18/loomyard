#!/usr/bin/env bash
# Usage: analyze.sh [runs.tsv] > tables.txt
# Prints the report's tables as markdown from runs.tsv (extract.sh) and producers.tsv (producers.sh), by analyze.jq.
# The periods are set by the commits that cut them, given in UTC:
# Before is a start before f08df88b, the first agent-turns change, and After a start after cfad47e6, the last one; a start between the two is Between.
# After splits at eea96fe2 (the batching hook), 119c1c05 (its widening) and 3085f451 (Discussion exploration by subagents).
# Needs bash, jq and git; reads only.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
runs=${1:-$here/runs.tsv}
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}

# utc <commit>: the commit's committer time in UTC, as the timing report writes a start.
utc() { TZ=UTC git -C "$prime" log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ "$1"; }

jq -R -s -r \
  --arg cut_before "$(utc f08df88b)" --arg cut_after "$(utc cfad47e6)" \
  --arg hook "$(utc eea96fe2)" --arg widened "$(utc 119c1c05)" --arg explore "$(utc 3085f451)" \
  --rawfile prod "${PRODUCERS:-$here/producers.tsv}" \
  -f "$here/analyze.jq" "$runs"
