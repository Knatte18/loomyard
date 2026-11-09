#!/usr/bin/env bash
# Usage, as the -exec program of a gate run, with the output directory and the shard count as its first two arguments:
#   lym gate test --wait --tier 1 ./internal/fabricengine -- -count=1 -timeout=21m -exec "<abs path>/shard-runner.sh <abs out dir> <shards>"
# go test then calls this script with the compiled test binary and the flags it would have passed the binary.
# With one shard the script runs the binary once, as go test would, and writes a timestamped test2json copy of its events.
# With N shards it runs N copies of the binary at once, each with a -test.run regex over its share of the top-level tests,
# and shares the tests out by longest-processing-time first over the per-test times in PERTEST (name<TAB>seconds, default below).
# Throughout the run it samples the machine into <out dir>:
#   cpu.tsv        epoch, then the /proc/stat jiffies user nice system idle iowait irq softirq steal, and the one-minute load
#   disk.tsv       epoch, then reads, read ms, writes, write ms, I/Os in flight and I/O ms, summed over the whole-disk devices
#   busy.txt       the five busiest processes every 10 s
#   git/           one trace2 event file per git process any shard started (the git processes, and the hub copies by their amend)
#   shard<K>.time  /usr/bin/time -v of shard K: CPU seconds of the shard and its children, wall, faults, context switches
#   wall.txt       epoch start and end of the test-binary phase, the load average before and after, and the shard walls
# Needs bash, awk, jq, xargs, /usr/bin/time and the test binary; writes only under <out dir>.
set -uo pipefail
export LC_ALL=C

out=$1
shards=$2
binary=$3
shift 3
pertest=${PERTEST:-$(dirname "$0")/../fabric-test-redundancy-2026-11-08/pertest.tsv}
mkdir -p "$out/git"
cp "$binary" "$out/fabric.test"

disks=$(lsblk -dno NAME,TYPE | awk '$2 == "disk" {print $1}' | paste -sd' ')

sample() {
  local tick=0
  while :; do
    now=$(date +%s.%N)
    printf '%s\t%s\t%s\n' "$now" "$(awk '/^cpu /{printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s", $2,$3,$4,$5,$6,$7,$8,$9}' /proc/stat)" "$(cut -d' ' -f1 /proc/loadavg)" >> "$out/cpu.tsv"
    awk -v now="$now" -v disks="$disks" 'BEGIN{n=split(disks,d," "); for(i=1;i<=n;i++) want[d[i]]=1}
      want[$3]{r+=$4; rm+=$7; w+=$8; wm+=$11; f+=$12; io+=$13}
      END{printf "%s\t%d\t%d\t%d\t%d\t%d\t%d\n", now, r, rm, w, wm, f, io}' /proc/diskstats >> "$out/disk.tsv"
    if [ $((tick % 10)) -eq 0 ]; then
      { printf '# %s\n' "$now"; ps -eo pcpu,pid,comm --sort=-pcpu | head -6; } >> "$out/busy.txt"
    fi
    tick=$((tick + 1))
    sleep 1
  done
}
sample &
sampler=$!
trap 'kill "$sampler" 2>/dev/null' EXIT

run_shard() {
  local index=$1
  shift
  GIT_TRACE2_EVENT="$out/git" /usr/bin/time -v -o "$out/shard$index.time" "$out/fabric.test" "$@" > "$out/shard$index.out" 2>&1
  echo $? > "$out/shard$index.exit"
}

load_before=$(cat /proc/loadavg)
start=$(date +%s.%N)

if [ "$shards" -le 1 ]; then
  GIT_TRACE2_EVENT="$out/git" /usr/bin/time -v -o "$out/shard1.time" "$out/fabric.test" "$@" \
    | tee >(go tool test2json -t -p internal/fabricengine > "$out/events.json")
  status=${PIPESTATUS[0]}
  sleep 1
else
  "$out/fabric.test" -test.list '^Test' | grep '^Test' > "$out/tests.txt"
  awk -v shards="$shards" -v pertest="$pertest" '
    BEGIN { while ((getline line < pertest) > 0) { split(line, f, "\t"); weight[f[1]] = f[2] } }
    { name[NR] = $0; w[NR] = ($0 in weight) ? weight[$0] : 0.05 }
    END {
      for (i = 1; i <= NR; i++) order[i] = i
      for (i = 2; i <= NR; i++) { key = order[i]; j = i - 1; while (j > 0 && w[order[j]] < w[key]) { order[j + 1] = order[j]; j-- } order[j + 1] = key }
      for (k = 1; k <= shards; k++) load[k] = 0
      for (i = 1; i <= NR; i++) {
        best = 1
        for (k = 2; k <= shards; k++) if (load[k] < load[best]) best = k
        load[best] += w[order[i]]
        print best "\t" name[order[i]]
      }
    }' "$out/tests.txt" > "$out/assignment.tsv"
  pids=()
  for index in $(seq 1 "$shards"); do
    regex="^($(awk -F'\t' -v k="$index" '$1 == k {print $2}' "$out/assignment.tsv" | paste -sd'|'))\$"
    run_shard "$index" "$@" -test.run "$regex" &
    pids+=($!)
  done
  wait "${pids[@]}"
  status=0
  for index in $(seq 1 "$shards"); do
    [ "$(cat "$out/shard$index.exit")" -eq 0 ] || status=1
    cat "$out/shard$index.out"
  done
fi

end=$(date +%s.%N)
{
  printf 'start\t%s\nend\t%s\nload_before\t%s\nload_after\t%s\n' "$start" "$end" "$load_before" "$(cat /proc/loadavg)"
  for file in "$out"/shard*.time; do
    printf '%s\t%s\n' "$(basename "$file")" "$(awk -F': ' '/Elapsed \(wall/{print $2}' "$file")"
  done
} > "$out/wall.txt"
exit "$status"
