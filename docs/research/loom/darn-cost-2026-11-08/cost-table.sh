#!/usr/bin/env bash
# Usage: cost-table.sh [out-dir]
# Joins parts.tsv, producers.tsv and run-table.tsv into one row per run, in <out-dir>/cost-table.tsv.
# Columns:
#   slug, recipe, landed, profile, stored_record, lines (changed lines of the landing commit, empty if none),
#   wall_min (the timing report's awake span: span less machine sleep),
#   discussion, plan, webster (the segments' cost in USD),
#   writer, writer_forks (the Darn segment's writer and the lookup subagents it forked),
#   reviewers (Code-Review reviewer and its forks), bouncer (Code-Review judge and seed), fixers (Code-Review fixer and its forks),
#   gate_fixer (Other, gate-fixer), landing (the Landing segment), other (the rest of the total), total,
#   rounds (Code-Burler-Review entries), writer_min (Darn-Write active minutes).
# The Orch segment holds no cost and is left out of the total.
# Needs bash and awk; reads only, apart from the table.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
out=${1:-$here}

awk -F'\t' -v OFS='\t' '
FILENAME ~ /run-table/ && FNR > 1 {
  slug = $1; recipe[slug] = $3; landed[slug] = $4; profile[slug] = $5; stored[slug] = $6
  wall[slug] = $11 / 60; lines[slug] = $14; seen[slug] = 1
  next
}
FILENAME ~ /parts/ && FNR > 1 {
  slug = $1; seg = $3; part = $4; usd = $5
  if (part == "-") {
    if (seg != "Orch") total[slug] += usd
    if (seg == "Discussion") discussion[slug] = usd
    if (seg == "Plan") plan[slug] = usd
    if (seg == "Webster") webster[slug] = usd
    if (seg == "Landing") landing[slug] = usd
    next
  }
  if (seg == "Darn" && part == "writer") writer[slug] = usd
  if (seg == "Darn" && part == "writer forks") wforks[slug] = usd
  if (seg == "Code-Review" && (part == "reviewer" || part == "reviewer forks")) reviewers[slug] += usd
  if (seg == "Code-Review" && (part == "judge" || part == "seed")) bouncer[slug] += usd
  if (seg == "Code-Review" && (part == "fixer" || part == "fixer forks")) fixers[slug] += usd
  if (seg == "Other" && part == "gate-fixer") gatefixer[slug] = usd
  next
}
FILENAME ~ /producers/ && FNR > 1 {
  if ($2 == "Code-Burler-Review") rounds[$1] = $3
  if ($2 == "Darn-Write") writermin[$1] = $6 / 60
  next
}
END {
  print "slug", "recipe", "landed", "profile", "stored_record", "lines", "wall_min", "discussion", "plan", "webster", \
    "writer", "writer_forks", "reviewers", "bouncer", "fixers", "gate_fixer", "landing", "other", "total", "rounds", "writer_min"
  for (slug in seen) {
    named = writer[slug] + wforks[slug] + reviewers[slug] + bouncer[slug] + fixers[slug] + gatefixer[slug] + landing[slug] + discussion[slug] + plan[slug] + webster[slug]
    printf "%s\t%s\t%s\t%s\t%s\t%s\t%.1f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%d\t%.1f\n", \
      slug, recipe[slug], landed[slug], profile[slug], stored[slug], lines[slug], wall[slug], discussion[slug], plan[slug], webster[slug], \
      writer[slug], wforks[slug], reviewers[slug], bouncer[slug], fixers[slug], gatefixer[slug], landing[slug], total[slug] - named, total[slug], \
      rounds[slug], writermin[slug]
  }
}
' "$here/run-table.tsv" "$here/parts.tsv" "$here/producers.tsv" | sort -t$'\t' -k3,3 -k1,1 > "$out/cost-table.body"
head -1 "$out/cost-table.body" > /dev/null
{ grep '^slug' "$out/cost-table.body" || true; grep -v '^slug' "$out/cost-table.body"; } > "$out/cost-table.tsv"
rm -f "$out/cost-table.body"
