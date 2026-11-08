#!/usr/bin/env bash
# Usage: group-means.sh [dir-with-tables]
# Writes group-means.tsv to the table directory (default: this directory): the mean Plan phase figures of three groups of runs,
# and the same figures per 1000 changed lines and per plan card.
#   audit    the 2026-10-24 audit's runs seeded 2026-10-19 or later with a known size, from audit-sizes.tsv;
#   earlier  the runs of plan-table.tsv that landed before 2026-11-04;
#   sample   the runs of plan-table.tsv that landed on or after 2026-11-05, the runs after the Discussion-Write stencil change.
# Per-1000-line and per-card figures divide the group's summed cost by its summed size, so a large run weighs by its size.
# The audit has no review or fix split, no fork session count and no Plan-Write time; those columns are empty for it.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)
dir=${1:-$here}
{
  printf 'group\truns\tcards\tchanged_lines\tplan_phase_usd\twriter_usd\twriter_forks_usd\twriter_plus_forks_usd\treview_usd\tfix_usd\tfork_sessions\trounds\tplan_write_elapsed_min\tplan_phase_per_kline\twriter_plus_forks_per_kline\twriter_per_kline\tplan_phase_per_card\twriter_plus_forks_per_card\n'
  awk -F'\t' -v OFS='\t' 'NR > 1 && $3 != "" { n++; c += $2; l += $3; p += $4; w += $5; f += $6; wf += $7; r += $8 }
    END { printf "audit\t%d\t%.1f\t%.0f\t%.2f\t%.2f\t%.2f\t%.2f\t\t\t\t%.1f\t\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\n", n, c / n, l / n, p / n, w / n, f / n, wf / n, r / n, p / l * 1000, wf / l * 1000, w / l * 1000, p / c, wf / c }' "$dir/audit-sizes.tsv"
  for group in earlier sample; do
    awk -F'\t' -v OFS='\t' -v group="$group" 'NR > 1 && (($2 < "2026-11-04") == (group == "earlier")) && $2 >= "2026-10-24" { n++; c += $3; l += $4; p += $5; w += $6; f += $7; wf += $8; rv += $9; fx += $10; fs += $12; r += $13; el += $14 }
      END { printf "%s\t%d\t%.1f\t%.0f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.1f\t%.1f\t%.1f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\n", group, n, c / n, l / n, p / n, w / n, f / n, wf / n, rv / n, fx / n, fs / n, r / n, el / n, p / l * 1000, wf / l * 1000, w / l * 1000, p / c, wf / c }' "$dir/plan-table.tsv"
  done
} > "$dir/group-means.tsv"
