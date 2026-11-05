#!/usr/bin/env bash
# usage.sh: print data/usage.txt, one line per finished cell under .scratch/bench/runs.
# Return code and wall time come from the cell's result.txt, tokens from its turn.completed event,
# verdict and finding count from its review file, and the window percentages from the cell's rollout.
ROOT=/home/knatte/Code/loomyard-LYXHUB/codex-review-bench
here=$(cd "$(dirname "$0")" && pwd)
for id in D1 D2 D3 D4 D5 D6 P1 P2 P3 P4; do
  for name in gpt-6.1-sol-medium gpt-6-astra-medium gpt-6-luna-low gpt-6-luna-medium; do
    cell=$ROOT/.scratch/bench/runs/$id/$name-r1
    [ -f "$cell/result.txt" ] || continue
    read -r rc wall < <(sed 's/wall_seconds=/wall=/' "$cell/result.txt" | head -1)
    done_line=$(grep '"turn.completed"' "$cell/events.jsonl" | tail -1)
    tokens() { grep -o "\"$1\":[0-9]*" <<<"$done_line" | cut -d: -f2; }
    verdict=none n=
    if [ -f "$cell/review.md" ]; then
      verdict=$(grep -m1 '^verdict:' "$cell/review.md" | cut -d' ' -f2)
      n=$(bash "$here/dump.sh" "$cell/review.md" | wc -l)
    fi
    rollout=$(grep -l "runs/$id/$name-r1/work\"" "$ROOT"/.scratch/codex-home/sessions/*/*/*/rollout-*.jsonl | tail -1)
    read -r five weekly < <(bash "$here/windows.sh" "$rollout")
    echo "$id $name-r1 $rc $wall in=$(tokens input_tokens) cached=$(tokens cached_input_tokens) out=$(tokens output_tokens) reason=$(tokens reasoning_output_tokens) verdict=$verdict n=$n five_hour_used_percent=${five:-} weekly_used_percent=${weekly:-}"
  done
done
