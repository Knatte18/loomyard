#!/usr/bin/env bash
# Usage: gate-runs.sh > gate-runs.tsv
# Prints one line per gate tier run this machine recorded, read from the hub's gate run records.
# Columns: start slug gate_key tier scope_form outcome wait_seconds wall_seconds launched launch_seconds elapsed_sum discovered passed skipped slots start_s
# wait_seconds is the time from asking for a slot to getting it, launched the test packages whose binary ran instead of answering from the test cache,
# elapsed_sum their summed package times, which run side by side, and start_s the start in seconds since the epoch.
# A record of the build step or of a cancelled call is printed with its own outcome, so a reader filters on the column.
# Needs bash, jq, git and find; reads only. GATE_RUNS names the record directory (default: the hub's, beside the prime).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
prime=${PRIME:-$(git -C "$root" worktree list --porcelain | awk 'NR == 1 { print $2 }')}
gate_runs=${GATE_RUNS:-$(dirname "$prime")/_bolt/.lym/gate/runs}

printf 'start\tslug\tgate_key\ttier\tscope_form\toutcome\twait_seconds\twall_seconds\tlaunched\tlaunch_seconds\telapsed_sum\tdiscovered\tpassed\tskipped\tslots\tstart_s\n'
find "$gate_runs" -name '*.json' -print0 | xargs -0 jq -r '
  def epoch: sub("\\.[0-9]+"; "") | sub("[+-][0-9][0-9]:[0-9][0-9]$"; "Z") | fromdateiso8601;
  def offset: (capture("(?<s>[+-])(?<h>[0-9][0-9]):(?<m>[0-9][0-9])$") // null) as $o
    | if $o == null then 0 else (($o.h | tonumber) * 3600 + ($o.m | tonumber) * 60) * (if $o.s == "-" then -1 else 1 end) end;
  def utc: (epoch - offset) | todateiso8601;
  def frac: (capture("\\.(?<f>[0-9]+)").f // "0") | ("0." + .) | tonumber;
  select(.start != null)
  | (.launches // []) as $l
  | [(.start | utc), (.slug // "-"), (.gate_key // "-"), (.tier // "-"), (.scope_form // "-"), (.outcome // "-"),
     (if .wait_start then ((.start | epoch) - (.start | offset) + (.start | frac) - (.wait_start | epoch) + (.wait_start | offset) - (.wait_start | frac)) * 10 | round / 10 else "-" end),
     ((.wall_seconds // 0) * 10 | round / 10),
     ($l | length),
     ($l | map(.launch_seconds // 0) | add // 0 | . * 10 | round / 10),
     ($l | map(.elapsed_seconds // 0) | add // 0 | . * 10 | round / 10),
     (.discovered // "-"), (.passed // "-"), (.skipped // "-"), (.slots // "-"),
     ((.start | epoch) - (.start | offset))]
  | @tsv' | sort
