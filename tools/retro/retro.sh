#!/usr/bin/env bash
# Usage: retro.sh <out-dir> <slug>...
# Reads the records and the Claude Code transcripts of the named landed runs and writes the retro figures to <out-dir>:
#   sessions.jsonl  one object per session and fork: slug session kind title row model cost
#                   turns calls lookup_turns serial_saved refusals gate_runs gate_fails
#   refusals.jsonl  one object per refused call: slug session kind turn tool refusal
#   rows.jsonl      one object per recipe row a run went through: slug idx producer outcome
#                   gate_attempts verify_runs verify_reworks routed_to
#   facts.jsonl     one object per review round cell: slug perch round column value
#   keys.jsonl      one object per ledger key that recurred across a perch's rounds: slug perch key rounds
#   missing.jsonl   one object per run with no archive tag or no transcript directory: slug what
#   usage.jsonl     one object per usage record of a session: slug session row role model cost
#   figures.md      the tables figures.jq renders from the files above
# A run's records are read at its newest archive tag; its transcripts from the transcript directory of its worktree.
# The transcript layout is Claude Code's:
# a run on another agent engine has no such directory and is listed in missing.jsonl.
# A session's row, model and cost come from the run's usage record of that session, and stay empty for a fork.
# Environment: PROJECTS names the transcript root (default ~/.claude/projects);
# HUB the directory holding the runs' worktrees (default: the parent of the prime worktree);
# RECORDS the records repository holding the archive tags (default: the prime worktree's path with -weft appended).
# Needs bash, jq, awk and git; writes only under <out-dir>.
set -euo pipefail
# The records' paths, inside an archive tag.
USAGE_DIR=_lym/usage
SHED_DIR=_lym/shed
REVIEWS_DIR=_lym/reviews
if [ $# -lt 2 ]; then
  echo "usage: retro.sh <out-dir> <slug>..." >&2
  exit 2
fi
out=$1
shift
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
projects=${PROJECTS:-$HOME/.claude/projects}
if [ -z "${HUB:-}" ] || [ -z "${RECORDS:-}" ]; then
  prime=$(git -C "$here" worktree list --porcelain | awk 'NR == 1 { print $2 }')
fi
hub=${HUB:-$(dirname "$prime")}
records=${RECORDS:-$prime-weft}

mkdir -p "$out"
for name in sessions refusals rows facts keys missing usage; do
  : > "$out/$name.jsonl"
done

# encode turns a worktree path into the name of its directory under the transcript root.
encode() { printf '%s' "$1" | sed 's/[^A-Za-z0-9]/-/g'; }

# transcript appends the rows of one transcript file to sessions.jsonl and refusals.jsonl.
transcript() { # file slug session kind
  local file=$1 slug=$2 session=$3 kind=$4
  jq -rs -f "$here/transcript.jq" "$file" |
    jq -cR --arg slug "$slug" --arg session "$session" --arg kind "$kind" --slurpfile usage "$out/usage.jsonl" '
      split("\t") as $f
      | ([$usage[] | select(.session == $session)] | first // {}) as $u
      | if $f[0] == "S" then
          {slug: $slug, session: $session, kind: $kind, title: $f[1], row: ($u.row // ""), model: ($u.model // ""), cost: ($u.cost // 0),
           turns: ($f[2] | tonumber), calls: ($f[3] | tonumber), lookup_turns: ($f[4] | tonumber), serial_saved: ($f[5] | tonumber),
           refusals: ($f[6] | tonumber), gate_runs: ($f[7] | tonumber), gate_fails: ($f[8] | tonumber)}
        else
          {slug: $slug, session: $session, kind: $kind, turn: ($f[1] | tonumber), tool: $f[2], refusal: $f[3]}
        end' |
    while IFS= read -r line; do
      if [[ $line == *'"refusal":'* ]]; then printf '%s\n' "$line" >> "$out/refusals.jsonl"; else printf '%s\n' "$line" >> "$out/sessions.jsonl"; fi
    done
}

for slug in "$@"; do
  tag=$(git -C "$records" for-each-ref --sort=-creatordate --format='%(refname:short)' "refs/tags/archive/$slug/" | head -n 1)
  if [ -z "$tag" ]; then
    jq -cn --arg slug "$slug" '{slug: $slug, what: "archive tag"}' >> "$out/missing.jsonl"
  else
    git -C "$records" ls-tree -r --name-only "$tag" -- "$USAGE_DIR" | { grep -E '/session-[^/]*\.json$' || true; } |
      while read -r path; do
        git -C "$records" show "$tag:$path" |
          jq -c --arg slug "$slug" '{slug: $slug, session: .session_id, row: (.row // ""), role: (.role // ""), model: (.model // ""), cost: (.cost_usd // 0)}'
      done >> "$out/usage.jsonl"
    if status=$(git -C "$records" show "$tag:$SHED_DIR/$slug/status.json" 2>/dev/null); then
      jq -c --arg slug "$slug" '
        [.history[]? | select(.at != null)] | to_entries[] | .key as $i | .value as $r
        | {slug: $slug, idx: $i, producer: $r.producer, outcome: $r.outcome, gate_attempts: ($r.gate_attempts // 0),
           verify_runs: ([$r.gate_spans[]? | select(.gate == "verify" and .kind == "gate")] | length),
           verify_reworks: ([$r.gate_spans[]? | select(.gate == "verify" and .kind == "rework")] | length),
           routed_to: ($r.routed_to // "")}' <<< "$status" >> "$out/rows.jsonl"
    fi
    # A facts file lists every round up to its own, so the newest file of a perch holds them all.
    git -C "$records" ls-tree -r --name-only "$tag" -- "$REVIEWS_DIR" | { grep -E '/round-[0-9]+-facts\.md$' || true; } |
      awk -F/ '{ dir = $0; sub(/\/[^\/]*$/, "", dir); n = $NF; gsub(/[^0-9]/, "", n); if (n + 0 >= best[dir] + 0) { best[dir] = n; path[dir] = $0 } } END { for (dir in path) print path[dir] }' |
      while read -r path; do
        perch=$(basename "$(dirname "$path")")
        git -C "$records" show "$tag:$path" | awk -f "$here/facts.awk" |
          jq -cR --arg slug "$slug" --arg perch "$perch" '
            split("\t") as $f
            | if $f[0] == "F" then {slug: $slug, perch: $perch, round: ($f[1] | tonumber), column: $f[2], value: $f[3]}
              else {slug: $slug, perch: $perch, key: $f[1], rounds: $f[2]} end' |
          while IFS= read -r line; do
            if [[ $line == *'"key":'* ]]; then printf '%s\n' "$line" >> "$out/keys.jsonl"; else printf '%s\n' "$line" >> "$out/facts.jsonl"; fi
          done
      done
  fi

  dir=$projects/$(encode "$hub/$slug")
  if [ ! -d "$dir" ]; then
    jq -cn --arg slug "$slug" '{slug: $slug, what: "transcript directory (Claude Code layout)"}' >> "$out/missing.jsonl"
    continue
  fi
  for f in "$dir"/*.jsonl; do
    [ -e "$f" ] || continue
    sid=$(basename "$f" .jsonl)
    transcript "$f" "$slug" "$sid" main
    for sub in "$dir/$sid"/subagents/*.jsonl; do
      [ -e "$sub" ] || continue
      transcript "$sub" "$slug" "$(basename "$sub" .jsonl)" fork
    done
  done
done

jq -rn -f "$here/figures.jq" \
  --slurpfile sessions "$out/sessions.jsonl" --slurpfile refusals "$out/refusals.jsonl" --slurpfile rows "$out/rows.jsonl" \
  --slurpfile facts "$out/facts.jsonl" --slurpfile keys "$out/keys.jsonl" --slurpfile missing "$out/missing.jsonl" \
  --slurpfile usage "$out/usage.jsonl" > "$out/figures.md"
