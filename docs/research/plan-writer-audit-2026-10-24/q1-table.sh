#!/usr/bin/env bash
# Builds $OUT/q1-table.tsv (Plan-writer cost, rounds and wall time per loom run) from the tokencount reports
# in $OUT/tc/<slug>.md (made by run-all.sh) and each run's seed.json in its archive tag in the records repo.
# The run list is the loom rows of docs/research/token-audit-2026-10-22/runcost.tsv plus $OUT/extra-loom.tsv when that file exists
# (loom archive tags missing from runcost.tsv; the committed baseline used one, a fresh checkout has none).
# Run from anywhere. Env: WEFT=<records repo> (default <repo>/../loomyard-weft); OUT=<work dir> (default <repo>/.scratch/plan-writer-audit/).
set -u
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"
WEFT="${WEFT:-$REPO/../loomyard-weft}"
OUT="${OUT:-$REPO/.scratch/plan-writer-audit}"; mkdir -p "$OUT"
RUNCOST="$REPO/docs/research/token-audit-2026-10-22/runcost.tsv"
cd "$OUT" || exit 1
out=q1-table.tsv
{ awk -F'\t' '$4=="loom"{print $2"\t"$3}' "$RUNCOST"
  [ -f extra-loom.tsv ] && awk -F'\t' '$4=="loom"{print $2"\t"$3}' extra-loom.tsv; } | sort -u -k1,1 > slug-tag.tsv
echo -e "start_utc\trun\tprofile_note\tplan_total_usd\twriter_usd\twriter_forks_usd\twriter_plus_forks_usd\twriter_fork_sessions\tplan_write_entries\tplan_write_elapsed\tplan_write_active\tplan_rounds" > $out
secs() { # "1h2m3s" -> seconds
  echo "$1" | awk '{h=0;m=0;s=0; if(match($0,/[0-9]+h/)){h=substr($0,RSTART,RLENGTH-1)} if(match($0,/[0-9]+m/)){m=substr($0,RSTART,RLENGTH-1)} if(match($0,/[0-9]+s/)){s=substr($0,RSTART,RLENGTH-1)} print h*3600+m*60+s}'
}
while IFS=$'\t' read -r slug tag; do
  f=tc/$slug.md
  [ -s $f ] || continue
  start=$(git -C "$WEFT" show $tag:_lyx/shed/$slug/seed.json 2>/dev/null | grep -o '"started_at": *"[^"]*"' | cut -d'"' -f4)
  [ -z "$start" ] && start="~$(git -C "$WEFT" log -1 --format=%cI $tag | cut -c1-16)"
  plan=$(grep -F '| **Plan** |' $f | head -1 | awk -F'|' '{gsub(/[ $]/,"",$3); print $3}')
  wr=$(awk -F'|' '/^\| \*\*Plan\*\*/{p=1;next} p&&/^\| \*\*/{p=0} p&&/└ writer \|/{gsub(/[ $]/,"",$3); print $3; exit}' $f)
  wf=$(awk -F'|' '/^\| \*\*Plan\*\*/{p=1;next} p&&/^\| \*\*/{p=0} p&&/└ writer forks/{gsub(/[ $]/,"",$3); print $3; exit}' $f)
  wfs=$(awk -F'|' '/^\| \*\*Plan\*\*/{p=1;next} p&&/^\| \*\*/{p=0} p&&/└ writer forks/{gsub(/ /,"",$8); print $8; exit}' $f)
  pw=$(grep -E '^\| Plan-Write \|' $f | head -1)
  pwe=$(echo "$pw" | awk -F'|' '{gsub(/ /,"",$3); print $3}'); pwel=$(echo "$pw" | awk -F'|' '{gsub(/ /,"",$4); print $4}'); pwa=$(echo "$pw" | awk -F'|' '{gsub(/ /,"",$6); print $6}')
  # rounds = reviewer sessions of the Plan segment (one fresh reviewer session per round)
  rr=$(awk -F'|' '/^\| \*\*Plan\*\*/{p=1;next} p&&/^\| \*\*/{p=0} p&&/└ reviewer \|/{gsub(/ /,"",$8); print $8; exit}' $f)
  [ -z "$wr" ] && continue
  # prefer the audit's own count (runcost.tsv column 18) where the run is in it
  rc=$(awk -F'\t' -v s=$slug '$2==s{print $18; exit}' "$RUNCOST")
  [ -n "$rc" ] && rr=$rc
  tot=$(awk -v a=${wr:-0} -v b=${wf:-0} 'BEGIN{printf "%.2f",a+b}')
  echo -e "$start\t$slug\t\t$plan\t$wr\t${wf:-0}\t$tot\t${wfs:-0}\t${pwe:-0}\t$(secs "$pwel")\t$(secs "$pwa")\t${rr:-0}" >> $out
done < slug-tag.tsv
# sort by start
{ head -1 $out; tail -n +2 $out | sort; } > q1-table.sorted.tsv && mv q1-table.sorted.tsv $out
