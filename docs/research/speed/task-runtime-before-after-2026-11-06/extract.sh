#!/usr/bin/env bash
# Usage: extract.sh [out-dir]
# Reads the hub's recorded data and writes runs.tsv, one row per landed run of the window, to out-dir (default: this directory):
# the timing report (phases, waits, sleep, active time), the convergence report (the commit that landed each run),
# `git diff --numstat` of that commit (changed lines), and the run's usage record in its archive tag (profile and stored-record flag).
# Columns: slug tag recipe start span sleep active esc halt parent_review pr_gate
#   setup discussion plan webster write review merge publish_finalize land paused other
#   profile stored_record has_record lines_added lines_deleted lines landing join kind
#   gate_wall gate_wait writer_gate_wall gate_runs (join-gate.sh)
# A "-" cell is a value the records do not hold: a phase the run has no row of, or a profile and stored-record flag of a run with no usage record.
# The raw report JSON goes to $SCRATCH (default <repo root>/.scratch/task-runtime-before-after).
# Environment: PRIME names the hub's prime worktree (default: the first worktree `git worktree list` shows), SINCE the window's first date (default 2026-09-01).
# Needs bash, jq, git and lym; reads only, apart from out-dir and the work directory.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
out=${1:-$here}
scratch=${SCRATCH:-$root/.scratch/task-runtime-before-after}
since=${SINCE:-2026-09-01}
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
weft=${WEFT:-$prime-weft}
mkdir -p "$out" "$scratch"

env -C "$prime" lym report timing --since "$since" --format json > "$scratch/timing.json"
env -C "$prime" lym report convergence --since "$since" --format json > "$scratch/convergence.json"
env -C "$prime" lym report cost --since "$since" --format json > "$scratch/cost.json"
env -C "$prime" lym gate report --since "$since" > "$scratch/gate.json"

# landing.tsv: tag, landing commit, join kind and kind of every run the convergence report maps to a commit.
jq -r '.landings[] | select(.tag != null) | [.tag, .commit, .join, .kind] | @tsv' "$scratch/convergence.json" > "$scratch/landing.tsv"

# A run the convergence report leaves unmapped, as every tapestry run is, takes the one first-parent commit of the parent branch committed up to 12 minutes before its last stamp.
# The mapped runs' commits lie 2 to 662 seconds before it, 65 seconds at the median.
# A commit another run holds is no candidate, and the passes repeat while a pass maps a run, so a commit two runs both reach goes to the one with no other.
# A run left with two or no candidates stays unmapped.
parent_branch=$(git -C "$prime" symbolic-ref --short HEAD)
jq -r '.runs[] | select(.state == "landed") | [.tag, .end] | @tsv' "$scratch/timing.json" > "$scratch/run-ends.tsv"
while :; do
  mapped_before=$(wc -l < "$scratch/landing.tsv")
  claimed=$(cut -f2 "$scratch/landing.tsv")
  while IFS=$'\t' read -r tag end; do
    grep -q "^$tag"$'\t' "$scratch/landing.tsv" && continue
    to=$(date -d "$end" +%s)
    candidates=$(git -C "$prime" log --first-parent --format=%H --since="@$((to - 720))" --until="@$to" "$parent_branch" | grep -vxF "$claimed" || true)
    if [ -n "$candidates" ] && [ "$(wc -l <<< "$candidates")" -eq 1 ]; then
      printf '%s\t%s\tend-time\t-\n' "$tag" "$candidates" >> "$scratch/landing.tsv"
      claimed+=$'\n'"$candidates"
    fi
  done < "$scratch/run-ends.tsv"
  [ "$(wc -l < "$scratch/landing.tsv")" -eq "$mapped_before" ] && break
done

# lines_of <commit>: added and deleted lines of the commit against its first parent, by the numstat the landing size record uses.
lines_of() {
  git -C "$prime" diff --numstat -M "$1^1" "$1" | awk -F'\t' '$1 != "-" { a += $1; d += $2 } END { printf "%d\t%d", a, d }'
}

{
  printf 'slug\ttag\trecipe\tstart\tspan\tsleep\tactive\tesc\thalt\tparent_review\tpr_gate\tsetup\tdiscussion\tplan\twebster\twrite\treview\tmerge\tpublish_finalize\tland\tpaused\tother\tprofile\tstored_record\thas_record\tlines_added\tlines_deleted\tlines\tlanding\tjoin\tkind\n'
  jq -r '.runs[] | select(.state == "landed") | [
      .slug, .tag, .recipe, .start, .span_seconds, .sleep_seconds, .active_seconds,
      .waits.escalation, .waits.halt, .waits.parent_review, .waits.pr_gate,
      .phases.setup, .phases.discussion, .phases.plan, .phases.webster, .phases.write, .phases.review,
      .phases.merge, .phases["publish-finalize"], .phases.land, .phases.paused, .phases.other
    ] | map(. // "-") | @tsv' "$scratch/timing.json" |
    while IFS=$'\t' read -r slug tag rest; do
      recipe=${rest%%$'\t'*}
      profile="-" stored="-"
      if usage=$(git -C "$weft" show "$tag:_lym/usage/run.json" 2>/dev/null); then
        profile=$(jq -r '.profile // ""' <<< "$usage")
        stored=$(jq -r '.stored_record' <<< "$usage")
      fi
      has_record=false
      git -C "$weft" cat-file -e "$tag:_lym/discussion/decision-record.md" 2>/dev/null && has_record=true
      landing="-" join="-" kind="-" added="-" deleted="-" lines="-"
      if row=$(awk -F'\t' -v t="$tag" '$1 == t { print $2 "\t" $3 "\t" $4; exit }' "$scratch/landing.tsv") && [ -n "$row" ]; then
        IFS=$'\t' read -r landing join kind <<< "$row"
        if size=$(lines_of "$landing" 2>/dev/null); then
          IFS=$'\t' read -r added deleted <<< "$size"
          lines=$((added + deleted))
        fi
      fi
      # The landed-size record, where the run holds one, is the size the pair itself measured and wins over the numstat above.
      if landed=$(git -C "$weft" show "$tag:_lym/usage/landed.json" 2>/dev/null); then
        added=$(jq -r '.lines_added' <<< "$landed")
        deleted=$(jq -r '.lines_deleted' <<< "$landed")
        lines=$((added + deleted))
      fi
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$tag" "$rest" "$profile" "$stored" "$has_record" "$added" "$deleted" "$lines" "$landing" "$join" "$kind"
    done
} > "$out/runs.tsv"

"$here/join-gate.sh" "$scratch/gate.json" "$out/runs.tsv"
