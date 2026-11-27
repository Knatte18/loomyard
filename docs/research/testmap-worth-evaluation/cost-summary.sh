#!/usr/bin/env bash
# Usage: cost-summary.sh <cost.tsv>
# Sums the output of cost.sh over the calls that named a gate key:
# per key the calls, their seconds, how many ran a report step, the seconds past the last tier,
# and the work and slot-wait seconds of the report steps;
# per step the sum, median and maximum of its work and slot waits and the test processes it started.
# Needs bash and awk; reads only.
awk -F'\t' '
  function add(k, v) { n[k]++; s[k] += v; a[k, n[k]] = v }
  function med(k,   i, j, t, m, b) {
    m = n[k]
    for (i = 1; i <= m; i++) b[i] = a[k, i] + 0
    for (i = 2; i <= m; i++) { t = b[i]; for (j = i - 1; j > 0 && b[j] > t; j--) b[j + 1] = b[j]; b[j + 1] = t }
    return m ? b[int((m + 1) / 2)] : 0
  }
  function top(k,   i, x) { for (i = 1; i <= n[k]; i++) if (a[k, i] + 0 > x) x = a[k, i] + 0; return x + 0 }
  function over(k, lim,   i, c) { for (i = 1; i <= n[k]; i++) if (a[k, i] + 0 > lim) c++; return c + 0 }
  BEGIN { split("map red mut", step, " ") }
  $3 != "-" {
    key = $3; calls[key]++; call[key] += $4
    if ($5 != "-") add("tail_" key, $5)
    any = 0
    for (i = 1; i <= 3; i++) {
      c = 6 + (i - 1) * 3
      if ($c == "-") continue
      add("wait_" step[i], $c); add("work_" step[i], $(c + 1)); procs[step[i]] += $(c + 2)
      work[key] += $(c + 1); wait[key] += $c; any = 1
    }
    if (any) { withstep[key]++; runs[$1] = 1 }
    allruns[$1] = 1
  }
  END {
    for (k in calls)
      printf "key %s calls=%d call_s=%d with_step=%d tail_sum=%d tail_med=%d step_work_s=%d step_wait_s=%d\n",
        k, calls[k], call[k], withstep[k], s["tail_" k], med("tail_" k), work[k], wait[k]
    for (i = 1; i <= 3; i++) {
      x = step[i]
      printf "step %s n=%d work_sum=%d work_med=%d work_max=%d wait_sum=%d wait_med=%d wait_over_60s=%d procs=%d\n",
        x, n["work_" x], s["work_" x], med("work_" x), top("work_" x), s["wait_" x], med("wait_" x), over("wait_" x, 60), procs[x]
    }
    for (r in allruns) na++
    for (r in runs) nr++
    printf "slugs with a keyed call=%d with a report step=%d\n", na, nr
  }' "$1" | sort
