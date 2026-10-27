#!/usr/bin/env bash
# Run the one fork-fan handler call: run-forkfan.sh <input-id> <effort> <label>
# One codex exec handler at the given effort spawns the lens forks itself; every rollout the call created is copied to the cell.
set -uo pipefail
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-v2-p4-bench
HARNESS=$ROOT/docs/research/codex-v2-p4-bench-2026-10-27/data/harness
id=$1 effort=$2 label=$3
model=gpt-6-astra
in=$ROOT/.scratch/bench/inputs/$id
cell=$ROOT/.scratch/bench/runs/$id/$label
rm -rf "$cell"; mkdir -p "$cell"
cp -a "$in/work" "$cell/work"
prompt=$(<"$in/fan/handler-fork.prompt.md")
prompt=${prompt//"$in/work"/"$cell/work"}
for f in "$cell"/work/.bench/instr/*.md; do
  c=$(<"$f"); printf '%s\n' "${c//"$in/work"/"$cell/work"}" > "$f"
done
printf '%s\n' "$prompt" > "$cell/prompt.md"
touch "$cell/.marker"
start=$(date +%s)
CODEX_HOME=$ROOT/.scratch/codex-home-$label timeout 3000 codex exec \
  -m "$model" -c "model_reasoning_effort=\"$effort\"" \
  -s workspace-write -C "$cell/work" --skip-git-repo-check --json \
  -o "$cell/last-message.txt" - <<<"$prompt" > "$cell/events.jsonl" 2> "$cell/stderr.txt"
rc=$?
end=$(date +%s)
echo "rc=$rc wall_seconds=$((end-start))" > "$cell/result.txt"
cp "$cell/work/.bench/out/review.md" "$cell/review.md" 2>/dev/null || echo "no review file" >> "$cell/result.txt"
mkdir -p "$cell/rollouts"
find "$ROOT/.scratch/codex-home-$label/sessions" -name 'rollout-*.jsonl' -newer "$cell/.marker" -exec cp {} "$cell/rollouts/" \;
ls "$cell/rollouts" >> "$cell/result.txt"
grep '"turn.completed"' "$cell/events.jsonl" | tail -1 > "$cell/turn-completed.json"
cat "$cell/result.txt"; cat "$cell/turn-completed.json"
