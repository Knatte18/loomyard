#!/usr/bin/env bash
# Usage: rows.sh [runs.tsv] > rows.tsv
# Prints one line per recipe row a landed run went through, read from the shed status and seed at the run's archive tag.
# A row's start is the stamp of the row before it, and the first row's start is the seed's started_at, so the rows of a run add up to its span.
# Columns: slug tag recipe run_start idx producer outcome start end seconds gate_seconds verify_seconds rework_seconds gate_attempts routed_to spans start_s end_s
# gate_seconds sums the row's gate spans of kind gate, verify_seconds those of them named verify, and rework_seconds the spans of kind rework,
# in which the row's agent repairs what a gate refused.
# spans lists the row's gate spans as "name/kind/start/end" joined by ";", the two times in milliseconds since the epoch, or "-" for a row with none.
# start_s and end_s repeat start and end in seconds since the epoch.
# A run whose tag holds no status file prints nothing.
# Needs bash, jq and git; reads only. WEFT names the records repository (default: the prime's, with -weft appended).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
runs=${1:-$here/runs.tsv}
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
weft=${WEFT:-$prime-weft}

printf 'slug\ttag\trecipe\trun_start\tidx\tproducer\toutcome\tstart\tend\tseconds\tgate_seconds\tverify_seconds\trework_seconds\tgate_attempts\trouted_to\tspans\tstart_s\tend_s\n'
awk -F'\t' -v OFS='\t' 'NR > 1 { print $1, $2, $3, $4 }' "$runs" |
  while IFS=$'\t' read -r slug tag recipe run_start; do
    status=$(git -C "$weft" show "$tag:_lym/shed/$slug/status.json" 2>/dev/null) || continue
    started=$(git -C "$weft" show "$tag:_lym/shed/$slug/seed.json" 2>/dev/null | jq -r '.started_at // empty' 2>/dev/null || true)
    jq -r --arg slug "$slug" --arg tag "$tag" --arg recipe "$recipe" --arg run_start "$run_start" --arg started "${started:-$run_start}" '
      def epoch: sub("\\.[0-9]+"; "") | fromdateiso8601;
      def span_seconds: ((.end | epoch) - (.start | epoch));
      def frac: capture("\\.(?<f>[0-9]+)").f // "0" | ("0." + .) | tonumber;
      def exact: (.end | epoch) + (.end | frac) - (.start | epoch) - (.start | frac);
      def millis: ((epoch + frac) * 1000 | round);
      [.history[] | select(.at != null)] as $h
      | range(0; $h | length) as $i
      | $h[$i] as $row
      | (if $i == 0 then $started else $h[$i - 1].at end) as $from
      | ($row.gate_spans // []) as $spans
      | [$slug, $tag, $recipe, $run_start, $i, $row.producer, $row.outcome, $from, $row.at,
         (($row.at | epoch) - ($from | epoch)),
         ($spans | map(select(.kind == "gate") | exact) | add // 0 | . * 10 | round / 10),
         ($spans | map(select(.kind == "gate" and .gate == "verify") | exact) | add // 0 | . * 10 | round / 10),
         ($spans | map(select(.kind == "rework") | exact) | add // 0 | . * 10 | round / 10),
         ($row.gate_attempts // 0), ($row.routed_to // "-"),
         (if ($spans | length) == 0 then "-" else ($spans | map("\(.gate)/\(.kind)/\(.start | millis)/\(.end | millis)") | join(";")) end),
         ($from | epoch), ($row.at | epoch)]
      | @tsv' <<< "$status"
  done
