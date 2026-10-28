#!/usr/bin/env bash
# Run one fork-fan handler call: run-forkfan.sh <input-id> <label> <effort>
# One codex exec handler at the given effort spawns the lens forks itself; every rollout the call created is copied to the cell.
# The run's PATH holds no lym or go: the handler reads and runs nothing that builds.
set -uo pipefail
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench
id=$1
model=gpt-6-astra label=$2 effort=$3
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
env -i HOME="$HOME" PATH=/usr/bin:/bin CODEX_HOME=$ROOT/.scratch/codex-home-$label timeout 3000 /home/knatte/.local/bin/codex exec \
  -m "$model" -c "model_reasoning_effort=\"$effort\"" \
  -s workspace-write -C "$cell/work" --skip-git-repo-check --json \
  -o "$cell/last-message.txt" - <<<"$prompt" > "$cell/events.jsonl" 2> "$cell/stderr.txt"
rc=$?
end=$(date +%s)
echo "rc=$rc wall_seconds=$((end-start))" > "$cell/result.txt"
cp "$cell/work/.bench/out/review.md" "$cell/review.md" 2>/dev/null || echo "no review file" >> "$cell/result.txt"
mkdir -p "$cell/lens"
cp "$cell"/work/.bench/out/review.lens-*.md "$cell/lens/" 2>/dev/null
mkdir -p "$cell/rollouts"
find "$ROOT/.scratch/codex-home-$label/sessions" -name 'rollout-*.jsonl' -newer "$cell/.marker" -exec cp {} "$cell/rollouts/" \;
ls "$cell/rollouts" >> "$cell/result.txt"
grep '"turn.completed"' "$cell/events.jsonl" | tail -1 > "$cell/turn-completed.json"
cat "$cell/result.txt"; cat "$cell/turn-completed.json"
