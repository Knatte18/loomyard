#!/usr/bin/env bash
# Usage: merge-saving.sh <role-table.tsv> <role-summary.tsv> > merge-saving.tsv
# What merging the serial turns would save, per role:
# role cost_usd strict_turns strict_per_session strict_weight_share strict_saving_usd class_weight_share class_saving_usd
# strict_turns is the turn audit's strict count of removable turns (role-summary.tsv m_saved_turns), class the upper bound (u_saved_turns);
# a removable turn is a single-call read, search, git-read, lym-read, quarry or skill turn after the first of a run of them.
# A saving is the role's list-price cost (role-table.tsv) times the removable turns' share of the role's weight (role-summary.tsv):
# the share is of the extractor's weight, a close but not equal measure to the list price, so a saving is an estimate.
# A merged turn is a little heavier in output and in the results it brings in at once; that is not subtracted.
set -euo pipefail
awk -F'\t' -v OFS='\t' '
FILENAME == ARGV[1] { if (FNR > 1) cost[$1] = $8; next }    # role-table.tsv: role sessions turns turns_per_session ctx_first ctx_mean ctx_peak cost_usd ...
FNR == 1 { next }
$1 in cost {                # role-summary.tsv: role sessions turns turns_per_session calls calls_per_turn single_share zero_share weight m_saved_turns m_saved_per_session m_saved_weight_share u_saved_turns u_saved_per_session u_saved_weight_share ...
  printf "%s\t%.2f\t%d\t%.1f\t%.2f\t%.2f\t%.2f\t%.2f\n", $1, cost[$1], $10, $11, $12, cost[$1] * $12, $15, cost[$1] * $15
}' "$1" "$2" | { printf 'role\tcost_usd\tstrict_turns\tstrict_per_session\tstrict_weight_share\tstrict_saving_usd\tclass_weight_share\tclass_saving_usd\n'; sort -t$'\t' -k1,1; }
