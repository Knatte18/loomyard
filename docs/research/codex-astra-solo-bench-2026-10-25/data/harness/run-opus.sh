#!/usr/bin/env bash
# Run one solo Opus review as an interactive shuttle session: run-opus.sh <input-id> <effort> <rep>
# The cell layout and path re-pointing match run.sh, so Opus reads the same prompt over the same snapshot.
set -uo pipefail
ROOT=/home/knatte/Code/loomyard-LYXHUB/codex-astra-solo-bench
id=$1 effort=$2 rep=${3:-1}
in=$ROOT/.scratch/bench/inputs/$id
cell=$ROOT/.scratch/bench/runs/$id/opus-$effort-r$rep
mkdir -p "$cell"
rm -rf "$cell/work"; cp -a "$in/work" "$cell/work"
prompt=$(<"$in/prompt.md")
prompt=${prompt//"$in/work"/"$cell/work"}
printf '%s\n' "$prompt" > "$cell/prompt.md"
for f in "$cell"/work/.bench/instr/*.md; do
  c=$(<"$f"); printf '%s\n' "${c//"$in/work"/"$cell/work"}" > "$f"
done
start=$(date +%s)
lyx shuttle run --model opus --effort "$effort" --prompt-file "$cell/prompt.md" \
  --output-file "$cell/work/.bench/out/review.md" --timeout 50m --role bench --round "$id" \
  > "$cell/outcome.json" 2> "$cell/stderr.txt"
rc=$?
end=$(date +%s)
echo "rc=$rc wall_seconds=$((end-start))" > "$cell/result.txt"
cp "$cell/work/.bench/out/review.md" "$cell/review.md" 2>/dev/null || echo "no review file" >> "$cell/result.txt"
cat "$cell/result.txt" "$cell/outcome.json"
