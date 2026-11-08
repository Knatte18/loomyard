#!/usr/bin/env bash
# Usage: per-darn.sh <dir> [cost-table.tsv]
# Reads <dir>/turn-costs.tsv, calls.tsv, gate-prompts.tsv and fixer-sessions.tsv and writes <dir>/per-darn.tsv, one row per darn run:
#   slug model effort sessions turns lines writer_usd cache_write_usd cache_read_usd output_usd input_usd forks_usd
#   research_usd write_usd gate_usd post_gate_edit_usd post_gate_other_usd reprompt_usd
#   gate_calls gate_failed gate_wait_s gate_rewrites gate_rewrite_usd other_rewrites other_rewrite_usd
#   reprompts fixer_sessions fixer_left fixer_usd fixer_wait_rewrites fixer_wait_rewrite_usd
# writer_usd is the darn writer's main session or sessions; forks_usd its lookup subagents; fixer_usd the cold gate fixer sessions.
# gate_calls counts the writer's own `lym gate` calls and gate_wait_s the sum of their durations;
# gate_rewrites and gate_rewrite_usd are the cache rewrites of the turns right after those calls.
# other_rewrites are the writer's rewrites after any other trigger except a "Gate findings recorded at" prompt;
# fixer_wait_rewrites are those after such a prompt (the wait while the verify gate and the cold fixer ran).
# lines is the changed lines of the run, from the optional cost-table.tsv of the darn cost measure (column `lines`), empty without it.
# Needs bash and awk; reads only, apart from per-darn.tsv.
set -euo pipefail
dir=$1
costtable=${2:-}
[ -n "$costtable" ] || costtable=/dev/null
awk -F'\t' -v OFS='\t' '
FILENAME == ARGV[1] && FNR > 1 { lines[$1] = $6; next }
# turn-costs.tsv: slug session kind role turn model effort ctx cost_input cost_cache_write cost_cache_read cost_output cost thinking ncalls stop
#   round activity gap_s trigger prev_ctx rewrite rewrite_tokens rewrite_usd
FILENAME == ARGV[2] && FNR > 1 {
  s = $1; seen[s] = 1
  if ($3 == "main" && $4 == "darn") {
    sessions[s, $2] = 1; turns[s]++; model[s] = $6; effort[s] = $7
    ci[s] += $9; cw[s] += $10; cr[s] += $11; co[s] += $12; wc[s] += $13
    act[s, $18] += $13
    if ($22 == 1) {
      if ($20 == "gatefindings") { fw[s]++; fwu[s] += $24 }
      else if ($20 == "gate" || $20 == "gotest") { gw[s]++; gwu[s] += $24 }
      else { ow[s]++; owu[s] += $24 }
    }
  } else if ($3 == "fork" && $4 == "darn+fork") fk[s] += $13
  else if ($3 == "main" && $4 == "gate-fixer") fx[s] += $13
  next
}
FILENAME == ARGV[3] && FNR > 1 && $3 == "main" && $4 == "darn" && $8 == "gate" { gc[$1]++; gd[$1] += $9; if ($11 == 1) gf[$1]++; next }
FILENAME == ARGV[4] && FNR > 1 { rp[$1]++; next }
FILENAME == ARGV[5] && FNR > 1 { fs[$1]++; fl[$1] += $9; next }
END {
  print "slug", "model", "effort", "sessions", "turns", "lines", "writer_usd", "cache_write_usd", "cache_read_usd", "output_usd", "input_usd", "forks_usd", \
    "research_usd", "write_usd", "gate_usd", "post_gate_edit_usd", "post_gate_other_usd", "reprompt_usd", \
    "gate_calls", "gate_failed", "gate_wait_s", "gate_rewrites", "gate_rewrite_usd", "other_rewrites", "other_rewrite_usd", \
    "reprompts", "fixer_sessions", "fixer_left", "fixer_usd", "fixer_wait_rewrites", "fixer_wait_rewrite_usd"
  for (s in seen) {
    if (!(s in turns)) continue
    n = 0; for (k in sessions) { split(k, p, SUBSEP); if (p[1] == s) n++ }
    printf "%s\t%s\t%s\t%d\t%d\t%s\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%d\t%d\t%.0f\t%d\t%.4f\t%d\t%.4f\t%d\t%d\t%d\t%.4f\t%d\t%.4f\n", \
      s, model[s], effort[s], n, turns[s], (s in lines ? lines[s] : ""), wc[s], cw[s], cr[s], co[s], ci[s], fk[s], \
      act[s, "research"], act[s, "write"], act[s, "gate"], act[s, "post-gate-edit"], act[s, "post-gate-other"], act[s, "reprompt"], \
      gc[s], gf[s], gd[s], gw[s], gwu[s], ow[s], owu[s], rp[s], fs[s], fl[s], fx[s], fw[s], fwu[s]
  }
}' "$costtable" "$dir/turn-costs.tsv" "$dir/calls.tsv" "$dir/gate-prompts.tsv" "$dir/fixer-sessions.tsv" | { read -r header; echo "$header"; sort; } > "$dir/per-darn.tsv"
