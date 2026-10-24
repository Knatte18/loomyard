#!/usr/bin/env bash
# Group means for $OUT/q1-table.tsv by period of the run's seed start (UTC; "~" = tag date when no seed):
#   A before 2026-10-20 (no full twill fan), B 2026-10-20 to c62274be5 (2026-10-22T13:38Z, full 8-lens fan), C after c62274be5 (5 lenses).
# Only runs with a Plan-Write row and at least one reviewer session. Writes $OUT/q1-summary.tsv and prints it.
# Run from anywhere. Env: OUT=<work dir> (default <repo>/.scratch/plan-writer-audit/).
set -u
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"
OUT="${OUT:-$REPO/.scratch/plan-writer-audit}"
cd "$OUT" || exit 1
awk -F'\t' 'NR>1 && $5!="" {
  s=$1; sub(/^~/,"",s)
  g=(s<"2026-10-15")?"A1 before 10-15 (solo writer, mostly no forks)":((s<"2026-10-20")?"A2 10-15..10-19 (early fan, 6 fork sessions)":((s<"2026-10-22T13:38")?"B 10-20..c62274be5 (full fan, 9 fork sessions)":"C after c62274be5 (five lenses, 6 fork sessions)"))
  n[g]++; w[g]+=$5; f[g]+=$6; t[g]+=$7; fs[g]+=$8; el[g]+=$10; ac[g]+=$11; r[g]+=$12; pt[g]+=$4
}
END{ printf "group\truns\tmean_plan_total\tmean_writer\tmean_forks\tmean_writer+forks\tmean_fork_sessions\tmean_PlanWrite_elapsed_min\tmean_PlanWrite_active_min\tmean_plan_rounds\n";
  for(g in n) printf "%s\t%d\t%.2f\t%.2f\t%.2f\t%.2f\t%.1f\t%.1f\t%.1f\t%.2f\n", g,n[g],pt[g]/n[g],w[g]/n[g],f[g]/n[g],t[g]/n[g],fs[g]/n[g],el[g]/n[g]/60,ac[g]/n[g]/60,r[g]/n[g] }' q1-table.tsv | sort > q1-summary.tsv
cat q1-summary.tsv
