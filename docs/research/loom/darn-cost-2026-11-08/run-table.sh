#!/usr/bin/env bash
# Usage: run-table.sh <timing.json> [out-dir]
# Builds <out-dir>/run-table.tsv, one row per run of parts.tsv, from `lym report timing --format json` (run from the
# hub's prime worktree), the records repo and the code repo.
# Columns:
#   slug, tag, recipe, landed (UTC date of the run's end), profile and stored_record (from runs.tsv, "-" before the
#   usage records existed), start, end, span_seconds, sleep_seconds, active_seconds (span less machine sleep),
#   review_seconds (the timing report's review phase), commit (short sha of the landing commit, "-" if none found),
#   lines (lines added plus deleted by that commit, empty if none).
# The run's tag is the newest archive tag of its slug in the timing report.
# The landing commit is the newest commit on main whose subject is the title of the change description archived at the
# tag and which was committed from a minute before the run's start to a minute after its end.
# Environment: WEFT names the records repo clone (default <repo root>/../loomyard-weft), CODE the code repo clone
# (default <repo root>/../loomyard).
# Needs bash, git, jq and awk; reads only, apart from the table.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
timing=$1
out=${2:-$here}
weft=${WEFT:-$root/../loomyard-weft}
code=${CODE:-$root/../loomyard}

printf 'slug\ttag\trecipe\tlanded\tprofile\tstored_record\tstart\tend\tspan_seconds\tsleep_seconds\tactive_seconds\treview_seconds\tcommit\tlines\n' > "$out/run-table.tsv"

# One line per run of parts.tsv that the timing report holds: its newest tag and times.
cut -f1 "$here/parts.tsv" | sort -u | grep -v '^slug$' > "$out/.slugs"
jq -r --rawfile slugs "$out/.slugs" '
  ($slugs | split("\n") | map(select(length > 0))) as $wanted
  | [.runs[] | select(.slug as $s | $wanted | index($s))]
  | group_by(.slug)
  | map(sort_by(.end) | last)
  | .[]
  | [.slug, .tag, .recipe, .start, .end, .span_seconds, .sleep_seconds, .active_seconds, (.phases.review // 0)]
  | @tsv' "$timing" |
  while IFS=$'\t' read -r slug tag recipe start end span sleep active review; do
    profile=-
    stored=-
    row=$(awk -F'\t' -v tag="$tag" '$1 == tag { print $5 "\t" $6 }' "$here/runs.tsv")
    if [ -n "$row" ]; then
      profile=${row%%$'\t'*}
      stored=${row##*$'\t'}
    fi
    title=$( (git -C "$weft" show "$tag:_lym/landing/summary.md" 2> /dev/null || git -C "$weft" show "$tag:_lyx/landing/summary.md" 2> /dev/null || true) | head -1 | sed 's/^# //')
    commit=-
    lines=
    if [ -n "$title" ]; then
      from=$(date -u -d "$start - 1 minute" +%s)
      to=$(date -u -d "$end + 1 minute" +%s)
      commit=$(git -C "$code" log main --format='%H%x09%ct%x09%s' --fixed-strings --grep="$title" |
        awk -F'\t' -v from="$from" -v to="$to" -v title="$title" '$3 == title && $2 >= from && $2 <= to { print $1; exit }')
      if [ -n "$commit" ]; then
        lines=$(git -C "$code" show --numstat --format= -m --first-parent "$commit" | awk '$1 != "-" { n += $1 + $2 } END { print n + 0 }')
        commit=${commit:0:8}
      else
        commit=-
      fi
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$slug" "$tag" "$recipe" "${end:0:10}" "$profile" "$stored" "$start" "$end" "$span" "$sleep" "$active" "$review" "$commit" "$lines" >> "$out/run-table.tsv"
  done
rm -f "$out/.slugs"
