#!/usr/bin/env bash
# Usage: join-gate.sh <gate.json> <runs.tsv>
# Sums `lym gate report` per slug into gate-by-slug.tsv beside runs.tsv (gate wall seconds and slot-wait seconds, in total and for the darn writer's own gate),
# then replaces runs.tsv with itself plus the columns gate_wall, gate_wait, writer_gate_wall and gate_runs.
# gate_wall is the summed wall time of the slug's tier runs and gate_wait their summed slot wait, both in seconds; the build step and abandoned calls are not in either.
# A run whose slug the gate report holds no row of gets "-" in all four columns.
# Two runs sharing a slug each get the slug's whole sum.
set -euo pipefail
gate=$1 runs=$2
dir=$(dirname "$runs")
jq -r '.rows | map(select(.slug != "")) | group_by(.slug)[] | [
    .[0].slug,
    (map(.wall_seconds) | add | round),
    (map(.wait_seconds) | add | round),
    (map(select(.gate_key == "darn-write") | .wall_seconds) | add // 0 | round),
    (map(.tier_runs) | add)
  ] | @tsv' "$gate" | sort > "$dir/gate-by-slug.tsv"
awk -F'\t' -v OFS='\t' '
  FILENAME == ARGV[1] { wall[$1] = $2; wait[$1] = $3; writer[$1] = $4; runs[$1] = $5; next }
  FNR == 1 { print $0, "gate_wall", "gate_wait", "writer_gate_wall", "gate_runs"; next }
  { if ($1 in wall) print $0, wall[$1], wait[$1], writer[$1], runs[$1]; else print $0, "-", "-", "-", "-" }
' "$dir/gate-by-slug.tsv" "$runs" > "$runs.joined"
mv "$runs.joined" "$runs"
