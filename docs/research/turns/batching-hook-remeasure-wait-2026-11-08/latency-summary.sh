#!/usr/bin/env bash
# Usage: latency-summary.sh <latency.tsv> > latency-summary.tsv
# Sums latency.sh rows by context band and call count: the calls and messages, and the median, 90th percentile and mean of latency_ms.
# The bands are below and at or above CONTEXT_TOKENS (default 150000, the hook's context_tokens).
# The groups are every call, the calls of a message that holds one call (single) and of a message that holds 2 or more (multi).
# A message is counted as 1 / ncalls per call, and its wait as the mean of its calls' latencies.
# added_s is the wait the band's messages add over the median latency of the below band's calls, in seconds:
# the sum over calls of (latency_ms - below median) / ncalls, so the parallel calls of one message count once.
# over_2s is the calls at 2000 ms or more, the hook's wait bound of 80 ticks of 25 ms.
# Columns: band group calls messages median_ms p90_ms mean_ms over_2s added_s
set -euo pipefail
awk -F'\t' -v OFS='\t' -v line="${CONTEXT_TOKENS:-150000}" '
function add(key, v, ncalls) {
  if (!(key in count)) order[++keys] = key
  values[key, ++count[key]] = v
  total[key] += v
  messages[key] += 1 / ncalls
  permessage[key] += v / ncalls
  if (v >= 2000) over[key]++
}
function sorted(key,    i, j, tmp) {
  for (i = 2; i <= count[key]; i++) {
    tmp = values[key, i]
    for (j = i - 1; j >= 1 && values[key, j] + 0 > tmp + 0; j--) values[key, j + 1] = values[key, j]
    values[key, j + 1] = tmp
  }
}
FNR == 1 { next }
{
  band = ($6 >= line) ? "above" : "below"
  add(band SUBSEP "all", $8, $7)
  add(band SUBSEP ($7 == 1 ? "single" : "multi"), $8, $7)
}
END {
  sorted("below" SUBSEP "all")
  base = values["below" SUBSEP "all", int((count["below" SUBSEP "all"] + 1) / 2)]
  print "band", "group", "calls", "messages", "median_ms", "p90_ms", "mean_ms", "over_2s", "added_s"
  for (k = 1; k <= keys; k++) {
    key = order[k]
    sorted(key)
    n = count[key]
    split(key, parts, SUBSEP)
    print parts[1], parts[2], n, sprintf("%.0f", messages[key]), values[key, int((n + 1) / 2)], values[key, int(0.9 * n + 0.999999)], sprintf("%.0f", total[key] / n), over[key] + 0, sprintf("%.0f", (permessage[key] - messages[key] * base) / 1000)
  }
}' "$1"
