#!/usr/bin/env bash
# Usage: resources.sh <out dir of one shard-runner.sh run>
# Prints what the run's samples say about whether the package was bound by CPU or by waiting:
#   the machine's CPU split (busy, iowait, idle) over the run and, when events.json exists, over phase 1 and phase 2 apart,
#   the disk's I/O time and operations per second,
#   each shard's own CPU seconds against its wall (user and system time include its git children),
#   the git processes the run started, the commands they ran, and the hub copies and scratch builds among them.
# A hub copy is counted by the `git commit --amend` its relocation always ends with, and a scratch build by `git init`.
# Needs awk, jq, find and xargs; reads only.
set -euo pipefail
export LC_ALL=C

out=$1
start=$(awk -F'\t' '$1 == "start" {print $2}' "$out/wall.txt")
end=$(awk -F'\t' '$1 == "end" {print $2}' "$out/wall.txt")
boundary=""
if [ -f "$out/events.json" ]; then
  boundary=$(date -d "$(jq -n -r 'first(inputs | select(.Action == "cont" and (.Test | contains("/") | not)) | .Time)' "$out/events.json")" +%s.%N)
fi

cpu_split() {
  local from=$1 to=$2 label=$3
  awk -F'\t' -v from="$from" -v to="$to" -v label="$label" '
    $1 >= from && $1 <= to {
      if (!seen) { for (i = 2; i <= 9; i++) first[i] = $i; seen = 1 }
      for (i = 2; i <= 9; i++) last[i] = $i
      samples++
    }
    END {
      for (i = 2; i <= 9; i++) delta[i] = last[i] - first[i]
      total = 0
      for (i = 2; i <= 9; i++) total += delta[i]
      idle = delta[5]; iowait = delta[6]
      printf "%-8s samples %d  busy %.1f%%  user+nice %.1f%%  system %.1f%%  irq+softirq %.1f%%  iowait %.1f%%  idle %.1f%%  steal %.1f%%\n", label, samples,
        100 * (total - idle - iowait) / total, 100 * (delta[2] + delta[3]) / total, 100 * delta[4] / total, 100 * (delta[7] + delta[8]) / total, 100 * iowait / total, 100 * idle / total, 100 * delta[9] / total
    }' "$out/cpu.tsv"
}

echo "== machine CPU (all processes on the machine, jiffies from /proc/stat)"
cpu_split "$start" "$end" whole
if [ -n "$boundary" ]; then
  cpu_split "$start" "$boundary" phase1
  cpu_split "$boundary" "$end" phase2
fi
echo "cores $(nproc)"

echo "== one-minute load in the samples: min, mean, max"
awk -F'\t' -v from="$start" -v to="$end" '$1 >= from && $1 <= to {n++; s += $10; if (n == 1 || $10 < lo) lo = $10; if ($10 > hi) hi = $10} END {printf "%.2f %.2f %.2f\n", lo, s / n, hi}' "$out/cpu.tsv"

echo "== disk (whole-disk devices): I/O busy milliseconds per second, reads and writes per second"
disk_split() {
  local from=$1 to=$2 label=$3
  awk -F'\t' -v from="$from" -v to="$to" -v label="$label" '
    $1 >= from && $1 <= to { if (!seen) { for (i = 1; i <= 7; i++) first[i] = $i; seen = 1 } for (i = 1; i <= 7; i++) last[i] = $i }
    END { span = last[1] - first[1]; printf "%-8s io-busy %.0f ms/s  reads %.0f/s  writes %.0f/s\n", label, (last[7] - first[7]) / span, (last[2] - first[2]) / span, (last[4] - first[4]) / span }' "$out/disk.tsv"
}
disk_split "$start" "$end" whole
if [ -n "$boundary" ]; then
  disk_split "$start" "$boundary" phase1
  disk_split "$boundary" "$end" phase2
fi

echo "== each shard: wall, user, system, CPU percent of its wall, maximum resident KiB, involuntary context switches"
for file in "$out"/shard*.time; do
  awk -F': ' -v name="$(basename "$file" .time)" '
    /User time/ {user = $2} /System time/ {sys = $2} /Percent of CPU/ {pct = $2} /Elapsed \(wall/ {wall = $2}
    /Maximum resident/ {rss = $2} /Involuntary context/ {ctx = $2}
    END {printf "%s  wall %s  user %s s  system %s s  cpu %s  rss %s  involuntary %s\n", name, wall, user, sys, pct, rss, ctx}' "$file"
done

echo "== git processes"
find "$out/git" -type f -print0 | xargs -0 cat | grep -h '"event":"cmd_name"' | jq -r '.name' | sort | uniq -c | sort -rn | awk '{total += $1; if (NR <= 12) print} END {print total " git processes in all"}'
find "$out/git" -type f -print0 | xargs -0 cat | grep -h '"event":"atexit"' | jq -r '.t_abs' | awk '{s += $1} END {printf "%.0f seconds of git wall time summed over processes\n", s}'
copies=$( (find "$out/git" -type f -print0 | xargs -0 grep -l '"argv":\["git","commit","--amend","--no-edit","--quiet","--",".lym-warp"\]' || true) | wc -l)
inits=$( (find "$out/git" -type f -print0 | xargs -0 grep -l '"argv":\["git","init"' || true) | wc -l)
echo "hub copies (relocation amends): $copies"
echo "git init processes (scratch hub builds, bare repos and clones): $inits"
