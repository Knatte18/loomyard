#!/usr/bin/env bash
# grid.sh <effort> <rep> <models...>: run every input for each model at one effort.
# A cell that finished with rc=0 is skipped; a failed or refused cell runs again.
# After each run the plan windows' used percentages go into the cell's result.txt,
# and the grid stops once either window shows STOP_PERCENT or more used.
ROOT=/home/knatte/Code/loomyard-LYXHUB/codex-review-bench
here=$(cd "$(dirname "$0")" && pwd)
STOP_PERCENT=70
marker=$ROOT/.scratch/bench/grid-start
effort=$1 rep=$2; shift 2
for model in "$@"; do
  for id in D1 D2 D3 D4 D5 D6 P1 P2 P3 P4; do
    cell=$ROOT/.scratch/bench/runs/$id/$model-$effort-r$rep
    grep -q '^rc=0' "$cell/result.txt" 2>/dev/null && continue
    echo "== $id $model $effort r$rep $(date +%T)"
    touch "$marker"
    bash "$here/run.sh" $id $model $effort $rep
    # Only a rollout written since the marker belongs to this run; a refused run may write none.
    rollout=$(find "$ROOT/.scratch/codex-home/sessions" -name 'rollout-*.jsonl' -newer "$marker" | sort | tail -1)
    [ -n "$rollout" ] || continue
    read -r five weekly < <(bash "$here/windows.sh" "$rollout")
    [ -n "${five:-}" ] || continue
    echo "five_hour_used_percent=$five weekly_used_percent=$weekly" | tee -a "$cell/result.txt"
    if [ "${five%.*}" -ge $STOP_PERCENT ] || [ "${weekly%.*}" -ge $STOP_PERCENT ]; then
      echo "stop: a plan window shows $STOP_PERCENT percent or more used" >&2
      exit 1
    fi
  done
done
