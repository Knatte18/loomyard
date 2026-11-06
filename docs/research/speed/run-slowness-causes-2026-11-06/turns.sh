#!/usr/bin/env bash
# Usage: turns.sh [pick.tsv] [out-dir]
# Reads the session transcripts of the picked runs and writes four tables to out-dir (default: this directory):
# sessions.tsv  slug session kind role first_ms last_ms
# turns.tsv     slug session kind role model effort start_ms first_ms end_ms input cache_read cache_write output thinking tools stop
# tools.tsv     slug session kind role name category start_ms end_ms result_chars
# prompts.tsv   slug session kind role at_ms source chars prompt
# A run's sessions are the transcripts in its worktree's project directory whose first entry lies inside the run's span, a minute of slack each way.
# A subagent transcript, a fork among them, is read with its parent session and takes the parent's role with "+sub" appended.
# A message a fork copied from its parent appears in both transcripts, and the tables keep its first appearance, the parent's.
# A session is named by the first eight characters of its id, a subagent by its parent's name, a slash and the last eight of its own.
# compact.jq and session.jq, beside this script, do the reading; times are milliseconds since the epoch.
# Needs bash, jq, awk and git; reads only, apart from out-dir. PROJECTS names Claude Code's projects directory (default ~/.claude/projects).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
pick=${1:-$here/pick.tsv}
out=${2:-$here}
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
projects=${PROJECTS:-$HOME/.claude/projects}
encoded=$(printf '%s' "$(dirname "$prime")" | tr -c 'A-Za-z0-9' '-')
all=$(mktemp)
trap 'rm -f "$all"' EXIT

# read_transcript <file> <slug> <session> <kind> <role>: the table lines of one transcript.
read_transcript() {
  jq -c -f "$here/compact.jq" "$1" | jq -s -r --arg slug "$2" --arg session "$3" --arg kind "$4" --arg role "$5" -f "$here/session.jq"
}

awk -F'\t' 'NR > 1 { print $1 "\t" $4 "\t" $5 }' "$pick" |
  while IFS=$'\t' read -r slug start end; do
    from=$(( ($(date -u -d "$start" +%s) - 60) * 1000 ))
    to=$(( ($(date -u -d "$end" +%s) + 60) * 1000 ))
    for main in "$projects/$encoded-$slug"/*.jsonl; do
      [ -f "$main" ] || continue
      id=$(basename "$main" .jsonl)
      session=${id:0:8}
      lines=$(read_transcript "$main" "$slug" "$session" main untitled)
      first=$(awk -F'\t' '$1 == "S" { print $6 }' <<< "$lines")
      [ -n "$first" ] && [ "$first" -ge "$from" ] && [ "$first" -le "$to" ] || continue
      role=$(awk -F'\t' '$1 == "S" { print $5 }' <<< "$lines")
      printf '%s\n' "$lines"
      for sub in "$projects/$encoded-$slug/$id"/subagents/*.jsonl; do
        [ -f "$sub" ] || continue
        agent=$(basename "$sub" .jsonl)
        read_transcript "$sub" "$slug" "$session/${agent: -8}" sub "$role+sub"
      done
    done
  done > "$all"

# table <letter> <key-column>: the lines of one table, the letter and, where key-column is set, that id column dropped after its first appearance is kept.
table() {
  awk -F'\t' -v OFS='\t' -v letter="$1" '$1 == letter { $1 = ""; print substr($0, 2) }' "$all" |
    if [ -n "${2:-}" ]; then
      sort -t$'\t' -k1,1 -k3,3 -k8,8n | awk -F'\t' -v OFS='\t' -v key="$2" '!seen[$1, $key]++ { $key = ""; sub(/\t\t/, "\t"); print }'
    else
      cat
    fi
}

{ printf 'slug\tsession\tkind\trole\tfirst_ms\tlast_ms\n'; table S; } > "$out/sessions.tsv"
{ printf 'slug\tsession\tkind\trole\tmodel\teffort\tstart_ms\tfirst_ms\tend_ms\tinput\tcache_read\tcache_write\toutput\tthinking\ttools\tstop\n'; table T 5; } > "$out/turns.tsv"
{ printf 'slug\tsession\tkind\trole\tname\tcategory\tstart_ms\tend_ms\tresult_chars\n'; table U 5; } > "$out/tools.tsv"
{ printf 'slug\tsession\tkind\trole\tat_ms\tsource\tchars\tprompt\n'; table P; } > "$out/prompts.tsv"
