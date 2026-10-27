#!/usr/bin/env bash
# Run one Codex review: run.sh <input-id> <model> <effort> <rep>
set -uo pipefail
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-astra-solo-high-bench
id=$1 model=$2 effort=$3 rep=${4:-1}
in=$ROOT/.scratch/bench/inputs/$id
cell=$ROOT/.scratch/bench/runs/$id/$model-$effort-r$rep
mkdir -p "$cell"
# Fresh work copy per run so a run never sees another run's output.
rm -rf "$cell/work"; cp -a "$in/work" "$cell/work"
prompt=$(<"$in/prompt.md")
# Re-point the prompt's paths from the input dir to the per-run copy.
prompt=${prompt//"$in/work"/"$cell/work"}
for f in "$cell"/work/.bench/instr/*.md; do
  c=$(<"$f"); printf '%s\n' "${c//"$in/work"/"$cell/work"}" > "$f"
done
start=$(date +%s)
CODEX_HOME=$ROOT/.scratch/codex-home timeout 3000 codex exec \
  -m "$model" -c "model_reasoning_effort=\"$effort\"" \
  -s workspace-write -C "$cell/work" --skip-git-repo-check --json \
  -o "$cell/last-message.txt" - <<<"$prompt" > "$cell/events.jsonl" 2> "$cell/stderr.txt"
rc=$?
end=$(date +%s)
echo "rc=$rc wall_seconds=$((end-start))" > "$cell/result.txt"
cp "$cell/work/.bench/out/review.md" "$cell/review.md" 2>/dev/null || echo "no review file" >> "$cell/result.txt"
rm -rf "$cell/work"
cat "$cell/result.txt"
