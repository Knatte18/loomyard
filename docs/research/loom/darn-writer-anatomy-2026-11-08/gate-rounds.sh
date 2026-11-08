#!/usr/bin/env bash
# Usage: gate-rounds.sh <dir>
# Reads <dir>/sessions.tsv, turns.tsv, prompts.tsv and turn-costs.tsv and writes two tables for the verify gate and the cold gate fixer:
#   <dir>/fixer-sessions.tsv: one row per gate-fixer session
#     slug session start_epoch end_epoch secs turns model cost left_findings
#     left_findings is 1 when a "Gate findings recorded at" prompt reached the darn writer within 600 seconds after the fixer ended,
#     that is, the fixer left findings for the writer; 0 when it cleared the gate or the run ended after it.
#   <dir>/gate-prompts.tsv: one row per "Gate findings recorded at" prompt the darn writer received
#     slug session ordinal prompt_epoch writer_idle_s fixer_secs fixer_cost fixer_model next_turn_rewrite rewrite_usd round_turns round_cost
#     writer_idle_s is the time from the end of the writer's last turn before the prompt to the prompt, the wait while the verify gate
#     and, when one ran, the cold fixer worked; fixer_* are empty when no fixer ended within 600 seconds before the prompt.
#     next_turn_rewrite and rewrite_usd come from the writer's first turn after the prompt (turn-costs.sh).
#     round_turns and round_cost are the writer's turns and cost from the prompt until the next such prompt or the session's end.
# Needs gawk (mktime); reads only, apart from the two tables.
set -euo pipefail
dir=$1
TZ=UTC awk -F'\t' -v OFS='\t' -v fixout="$dir/fixer-sessions.tsv" -v promptout="$dir/gate-prompts.tsv" '
function ep(s) { gsub(/[-T:Z]/, " ", s); sub(/\.[0-9]+/, "", s); return mktime(s) }
BEGIN {
  print "slug", "session", "start_epoch", "end_epoch", "secs", "turns", "model", "cost", "left_findings" > fixout
  print "slug", "session", "ordinal", "prompt_epoch", "writer_idle_s", "fixer_secs", "fixer_cost", "fixer_model", "next_turn_rewrite", "rewrite_usd", "round_turns", "round_cost" > promptout
}
# sessions.tsv: slug session role kind first_ts last_ts model title turns
FILENAME == ARGV[1] && FNR > 1 && $3 == "gate-fixer" && $4 == "main" {
  nf[$1]++; i = nf[$1]
  fsess[$1, i] = $2; fstart[$1, i] = ep($5); fend[$1, i] = ep($6); fturns[$1, i] = $9; fmodel[$1, i] = $7
  next
}
# turns.tsv: slug session kind role turn ts_first ts_last ...
FILENAME == ARGV[2] && FNR > 1 && $3 == "main" && $4 == "darn" {
  if ($5 + 0 > nt[$2]) nt[$2] = $5 + 0
  tfirst[$2, $5] = $6; tlast[$2, $5] = $7
  next
}
# turn-costs.tsv: slug session kind role turn model effort ctx ... cost(13) ... round(17) ... rewrite(22) rewrite_tokens rewrite_usd(24)
FILENAME == ARGV[3] && FNR > 1 {
  if ($3 == "main" && $4 == "darn") {
    tcost[$2, $5] = $13; tround[$2, $5] = $17; trewrite[$2, $5] = $22; trusd[$2, $5] = $24
    rcost[$2, $17] += $13; rturns[$2, $17]++
  }
  if ($3 == "main" && $4 == "gate-fixer") fcost[$2] += $13
  next
}
# prompts.tsv: slug session kind role ts chars head
FILENAME == ARGV[4] && FNR > 1 && $3 == "main" && $4 == "darn" && $7 ~ /^Gate findings recorded at/ {
  k = ++ord[$2]
  slug = $1; sess = $2; ts = $5 + 0
  prev = 0
  for (t = 1; t <= nt[sess]; t++) if (tfirst[sess, t] < ts) prev = t
  idle = prev ? ts - tlast[sess, prev] : ""
  best = 0; bi = 0
  for (j = 1; j <= nf[slug]; j++) if (fend[slug, j] <= ts + 5 && ts - fend[slug, j] < 600 && fend[slug, j] > best) { best = fend[slug, j]; bi = j }
  if (bi) left[slug, bi] = 1
  nx = prev + 1
  printf "%s\t%s\t%d\t%.0f\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%.4f\n", slug, sess, k, ts, (idle == "" ? "" : sprintf("%.0f", idle)),
    (bi ? fend[slug, bi] - fstart[slug, bi] : ""), (bi ? sprintf("%.4f", fcost[fsess[slug, bi]]) : ""), (bi ? fmodel[slug, bi] : ""),
    ((sess, nx) in trewrite ? trewrite[sess, nx] : ""), ((sess, nx) in trusd ? trusd[sess, nx] : ""), rturns[sess, k], rcost[sess, k] >> promptout
  next
}
END {
  for (key in fsess) {
    split(key, p, SUBSEP)
    printf "%s\t%s\t%d\t%d\t%d\t%s\t%s\t%.4f\t%d\n", p[1], fsess[key], fstart[key], fend[key], fend[key] - fstart[key], fturns[key], fmodel[key], fcost[fsess[key]], (key in left ? 1 : 0) >> fixout
  }
}' "$dir/sessions.tsv" "$dir/turns.tsv" "$dir/turn-costs.tsv" "$dir/prompts.tsv"
