#!/usr/bin/env bash
# Usage: plan-table.sh [out-dir]
# Builds <out-dir>/plan-table.tsv, one row per run of plan-slugs.txt: the Plan phase's cost by part, its rounds and time,
# and the run's size (plan cards and changed lines), from tools/tokencount and the records repo.
# Columns:
#   slug, landed (UTC date of the run's newest archive tag), cards, changed_lines,
#   plan_phase_usd (writer, forks, review, fix: the Plan segment), writer_usd, writer_forks_usd, writer_plus_forks_usd,
#   review_usd (reviewer and its forks), fix_usd (fixer and its forks), other_usd (judge and seed),
#   writer_fork_sessions, rounds (reviewer sessions), plan_write_elapsed_min, plan_write_active_min.
# cards counts _lym/plan/NN-*.md (_lyx before the rename) at the archive tag; changed_lines is the insertions plus deletions of the landing commit,
# the newest commit on the code repo's main titled after the change description archived at the tag.
# Environment: OUT names the work directory (default <repo root>/.scratch/writer-cost-remeasure), WEFT the records repo clone (default <repo root>/../loomyard-weft).
# Needs bash, git, go with cgo, and awk; reads only, apart from the table and the work directory.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
out=${1:-$here}
work=${OUT:-$root/.scratch/writer-cost-remeasure}
weft=${WEFT:-$root/../loomyard-weft}
mkdir -p "$out" "$work/tc"
[ -x "$work/tokencount-bin" ] || (cd "$root" && go build -o "$work/tokencount-bin" ./tools/tokencount)

# Seconds of a duration such as 1h2m3s.
secs() {
  awk -v d="$1" 'BEGIN { h = m = s = 0; if (match(d, /[0-9]+h/)) h = substr(d, RSTART, RLENGTH - 1); if (match(d, /[0-9]+m/)) m = substr(d, RSTART, RLENGTH - 1); if (match(d, /[0-9]+s/)) s = substr(d, RSTART, RLENGTH - 1); print h * 3600 + m * 60 + s }'
}

printf 'slug\tlanded\tcards\tchanged_lines\tplan_phase_usd\twriter_usd\twriter_forks_usd\twriter_plus_forks_usd\treview_usd\tfix_usd\tother_usd\twriter_fork_sessions\trounds\tplan_write_elapsed_min\tplan_write_active_min\n' > "$out/plan-table.tsv"
while read -r slug; do
  [ -n "$slug" ] || continue
  report=$work/tc/$slug.md
  # tokencount resolves the hub from the worktree root, so it runs from there.
  if [ ! -s "$report" ]; then
    # A run whose transcripts are not on this machine yields no row.
    (cd "$root" && "$work/tokencount-bin" -detail -out "$report" "$slug" > /dev/null) || { echo "plan-table: no row for $slug" >&2; continue; }
  fi
  tag=$(git -C "$weft" tag -l "archive/$slug/*" --sort=-creatordate | head -1)
  landed=$(git -C "$weft" log -1 --format=%cI "$tag" | cut -c1-10)
  # Runs archived before the records directory was renamed keep their files under _lyx.
  cards=$(git -C "$weft" ls-tree -r --name-only "$tag" | grep -cE '^_ly[mx]/plan/[0-9]+-' || true)
  title=$( (git -C "$weft" show "$tag:_lym/landing/summary.md" 2> /dev/null || git -C "$weft" show "$tag:_lyx/landing/summary.md") | head -1 | sed 's/^# //')
  commit=$(git -C "$root" log main --format=%h --fixed-strings --grep="$title" | head -1)
  lines=$(git -C "$root" diff --shortstat "$commit^1" "$commit" | awk '{ print $4 + $6 }')
  # Parts of the Plan segment: the rows between the Plan row and the next segment row.
  part() { awk -F'|' -v want="$1" -v col="$2" '/^\| \*\*Plan\*\*/ { p = 1; next } p && /^\| \*\*/ { exit } p { n = $2; gsub(/^ *└ *| *$/, "", n); if (n == want) { v = $col; gsub(/[ $]/, "", v); print v; exit } }' "$report"; }
  plan=$(awk -F'|' '/^\| \*\*Plan\*\*/ { v = $3; gsub(/[ $*]/, "", v); print v; exit }' "$report")
  writer=$(part writer 3); wforks=$(part "writer forks" 3); wfs=$(part "writer forks" 8)
  reviewer=$(part reviewer 3); rforks=$(part "reviewer forks" 3); rounds=$(part reviewer 8)
  fixer=$(part fixer 3); fforks=$(part "fixer forks" 3)
  judge=$(part judge 3); seed=$(part seed 3)
  pw=$(grep -E '^\| Plan-Write \|' "$report" | head -1)
  elapsed=$(secs "$(echo "$pw" | awk -F'|' '{ gsub(/ /, "", $4); print $4 }')")
  active=$(secs "$(echo "$pw" | awk -F'|' '{ gsub(/ /, "", $6); print $6 }')")
  awk -v OFS='\t' -v slug="$slug" -v landed="$landed" -v cards="$cards" -v lines="$lines" -v plan="$plan" -v w="${writer:-0}" -v wf="${wforks:-0}" -v wfs="${wfs:-0}" \
    -v rv="${reviewer:-0}" -v rf="${rforks:-0}" -v rounds="${rounds:-0}" -v fx="${fixer:-0}" -v ff="${fforks:-0}" -v j="${judge:-0}" -v sd="${seed:-0}" -v el="$elapsed" -v ac="$active" \
    'BEGIN { printf "%s\t%s\t%d\t%d\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%d\t%d\t%.1f\t%.1f\n", slug, landed, cards, lines, plan, w, wf, w + wf, rv + rf, fx + ff, j + sd, wfs, rounds, el / 60, ac / 60 }' >> "$out/plan-table.tsv"
done < "$here/plan-slugs.txt"
