#!/usr/bin/env bash
# Usage: run-all.sh [out-dir]
# Rebuilds every table of the report from the Claude Code transcripts on this machine:
# session-costs.tsv, turns.tsv, calls.tsv, sessions.tsv, role-summary.tsv, where.tsv, merge-kinds.tsv, kinds-total.tsv,
# role-table.tsv, merge-saving.tsv, rounds.tsv, fixer-cost.tsv.
# out-dir defaults to this directory.
# Environment: PROJECTS names the transcript root (default ~/.claude/projects),
# SCRATCH the work directory (default <repo root>/.scratch/review-turn-audit).
# Needs bash, jq and awk; reads only.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
out=${1:-$here}
# The repository root is looked up only for the default, so a staged copy outside the repository runs when SCRATCH is set.
scratch=${SCRATCH:-$(git -C "$here" rev-parse --show-toplevel)/.scratch/review-turn-audit}
audit=$here/../turn-audit-2026-10-28
mkdir -p "$out" "$scratch/extract"

"$here/session-costs.sh" > "$out/session-costs.tsv"

# The turn audit's extractor reads slugs.txt beside itself, so run a copy of it next to this run's slugs.
cp "$audit/extract-turns.sh" "$scratch/extract/extract-turns.sh"
cp "$here/slugs.txt" "$scratch/extract/slugs.txt"
CUTOFF=2026-10-27 "$scratch/extract/extract-turns.sh" "$scratch/extract/all"

# Keep the Discussion and Plan review segment: reviewer, fixer and the bouncer's seed and judge, with their forks.
# A fork transcript opens with its parent's spawning Agent message, replayed; the parent already counts it, so turn 1 of every fork is dropped.
# Stop if a fork's first turn is anything else, since the drop would then cut real work.
# A reviewer or fixer session that ran on Sonnet takes "@sonnet" after its role, so its rows stay apart from the Opus sessions of the same role.
segment='^(discussion|plan)-(burler-review|burler-fix|bouncer-seed|bouncer-judge)(\+fork)?$'
awk -F'\t' -v segment="$segment" '
  $3 == "fork" && $5 == 1 && $4 ~ segment && $9 != "agent" { bad = 1; print "fork " $2 " opens with a " $9 " call, not the replayed Agent call" > "/dev/stderr" }
  END { exit bad }' "$scratch/extract/all/calls.tsv"
for table in turns calls; do
  awk -F'\t' -v OFS='\t' -v segment="$segment" '
    FILENAME == ARGV[1] { if (FNR > 1 && $7 == "claude-sonnet-5-5") sonnet[$2] = 1; next }
    FNR == 1 { print; next }
    $4 !~ segment || ($3 == "fork" && $5 == 1) { next }
    {
      if (($2 in sonnet) && $4 ~ /burler-(review|fix)/) sub(/(\+fork)?$/, "@sonnet&", $4)
      print
    }' "$out/session-costs.tsv" "$scratch/extract/all/$table.tsv" > "$out/$table.tsv"
done

"$audit/session-metrics.sh" "$out" > "$out/sessions.tsv"
"$audit/role-summary.sh" "$out/sessions.tsv" > "$out/role-summary.tsv"
"$here/where-turns-go.sh" "$out" > "$out/where.tsv"
"$audit/merge-kinds.sh" "$out" > "$out/merge-kinds.tsv"
"$audit/kinds-total.sh" "$out/merge-kinds.tsv" > "$out/kinds-total.tsv"
"$here/role-table.sh" "$out" > "$out/role-table.tsv"
"$here/merge-saving.sh" "$out/role-table.tsv" "$out/role-summary.tsv" > "$out/merge-saving.tsv"
"$here/rounds-table.sh" "$out" > "$out/rounds.tsv"
"$here/fixer-cost.sh" "$out/session-costs.tsv" > "$out/fixer-cost.tsv"
