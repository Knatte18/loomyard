#!/usr/bin/env bash
# Usage: parse-tokencount.sh <tokencount-report.md> [out-dir]
# Splits the report `tokencount -since <date>` writes into two tables in <out-dir>:
#   parts.tsv:     slug, recipe, segment, part, cost_usd, active_seconds
#                  (a segment's own row has the part "-"; the Orch rows and unfinished figures are as the report shows them)
#   producers.tsv: slug, producer, entries, elapsed_seconds, idle_seconds, active_seconds
# A run's recipe comes from the report's "Cost per run" table; sections of other headings are skipped.
# Needs bash and awk; reads only, apart from the two tables.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
report=$1
out=${2:-$here}
mkdir -p "$out"

awk -F'|' -v parts="$out/parts.tsv" -v producers="$out/producers.tsv" '
function trim(s) { gsub(/^ +| +$/, "", s); return s }
# Seconds of a duration such as 1h2m3s; an empty cell is 0.
function secs(d,   h, m, s) {
  h = m = s = 0
  if (match(d, /[0-9]+h/)) h = substr(d, RSTART, RLENGTH - 1)
  if (match(d, /[0-9]+m/)) m = substr(d, RSTART, RLENGTH - 1)
  if (match(d, /[0-9]+s/)) s = substr(d, RSTART, RLENGTH - 1)
  return h * 3600 + m * 60 + s
}
BEGIN {
  print "slug\trecipe\tsegment\tpart\tcost_usd\tactive_seconds" > parts
  print "slug\tproducer\tentries\telapsed_seconds\tidle_seconds\tactive_seconds" > producers
}
/^## Cost per run/ { mode = "cost"; next }
/^## / {
  name = substr($0, 4)
  mode = (name in recipe) ? "run" : "skip"
  slug = name
  kind = ""
  next
}
/^### Producers/ { kind = "producers"; next }
mode == "cost" && /^\| / && $3 ~ /^ *(darn|tapestry|motif|raddle)/ { recipe[trim($2)] = trim($3); next }
mode == "run" && kind == "" && /^\| \*\*/ {
  seg = trim($2); gsub(/\*\*|, \$[0-9.]+ per run/, "", seg)
  cost = trim($3); gsub(/[$*]/, "", cost)
  printf "%s\t%s\t%s\t-\t%s\t%d\n", slug, recipe[slug], seg, cost, secs(trim($5)) > parts
  next
}
mode == "run" && kind == "" && /^\| └ / {
  part = trim($2); sub(/^└ */, "", part)
  cost = trim($3); gsub(/[$*]/, "", cost)
  printf "%s\t%s\t%s\t%s\t%s\t%d\n", slug, recipe[slug], seg, part, cost, secs(trim($5)) > parts
  next
}
mode == "run" && kind == "producers" && /^\| / && $2 !~ /producer|---/ {
  printf "%s\t%s\t%s\t%d\t%d\t%d\n", slug, trim($2), trim($3), secs(trim($4)), secs(trim($5)), secs(trim($6)) > producers
}
' "$report"
