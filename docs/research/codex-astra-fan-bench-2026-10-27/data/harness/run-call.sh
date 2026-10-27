#!/usr/bin/env bash
# Run one Codex call of the fan: run-call.sh <input-id> <lens-affected-set|lens-guard-reach|lens-way-forward|handler>
# Each call gets a fresh copy of the snapshot; the handler's copy also holds the three lens outputs.
set -uo pipefail
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-astra-fan-bench
HARNESS=$ROOT/docs/research/codex-astra-fan-bench-2026-10-27/data/harness
id=$1 call=$2
model=gpt-6-astra effort=medium
in=$ROOT/.scratch/bench/inputs/$id
runs=$ROOT/.scratch/bench/runs/$id
cell=$runs/$call
rm -rf "$cell"; mkdir -p "$cell"
cp -a "$in/work" "$cell/work"
prompt=$(<"$in/fan/$call.prompt.md")
prompt=${prompt//"$in/work"/"$cell/work"}
for f in "$cell"/work/.bench/instr/*.md; do
  c=$(<"$f"); printf '%s\n' "${c//"$in/work"/"$cell/work"}" > "$f"
done
if [ "$call" = handler ]; then
  mkdir -p "$cell/work/.bench/lens"
  for l in affected-set guard-reach way-forward; do
    cp "$runs/lens-$l/last-message.txt" "$cell/work/.bench/lens/$l.md"
  done
fi
printf '%s\n' "$prompt" > "$cell/prompt.md"
start=$(date +%s)
CODEX_HOME=$ROOT/.scratch/codex-home timeout 3000 codex exec \
  -m "$model" -c "model_reasoning_effort=\"$effort\"" \
  -s workspace-write -C "$cell/work" --skip-git-repo-check --json \
  -o "$cell/last-message.txt" - <<<"$prompt" > "$cell/events.jsonl" 2> "$cell/stderr.txt"
rc=$?
end=$(date +%s)
echo "rc=$rc wall_seconds=$((end-start))" > "$cell/result.txt"
cp "$cell/work/.bench/out/review.md" "$cell/review.md" 2>/dev/null || echo "no review file" >> "$cell/result.txt"
rollout=$(ls -t "$ROOT"/.scratch/codex-home/sessions/*/*/*/rollout-*.jsonl | head -1)
windows=$(bash "$HARNESS/windows.sh" "$rollout" | head -1)
tok=$(grep '"turn.completed"' "$cell/events.jsonl" | tail -1)
echo "$tok" > "$cell/turn-completed.json"
echo "windows=$windows rollout=$rollout" >> "$cell/result.txt"
rm -rf "$cell/work"
cat "$cell/result.txt"; cat "$cell/turn-completed.json"
