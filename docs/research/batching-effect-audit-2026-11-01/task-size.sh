#!/usr/bin/env bash
# Usage: task-size.sh > sizes.tsv
# One row per run of runs.tsv, the size of the task as its Discussion and Plan left it:
# slug archive_tag decision_record_bytes plan_cards plan_bytes
# plan_cards counts the numbered card files under _lym/plan/ except the overview, and plan_bytes their size.
# A run with an archive tag is read from the records repo (WEFT, default <repo root>/../loomyard-weft);
# a run whose tag column is "-" has not landed and is read from its weft worktree <hub>/<slug>-weft.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../../.." && pwd)
weft=${WEFT:-$root/../loomyard-weft}
hub=$(cd "$root/.." && pwd)

printf 'slug\tarchive_tag\tdecision_record_bytes\tplan_cards\tplan_bytes\n'
mkdir -p "$root/.scratch/batching-effect-audit"
while IFS=$'\t' read -r slug tag; do
  scratch=$(mktemp -d "$root/.scratch/batching-effect-audit/size-$slug.XXXXXX")
  if [[ $tag == "-" ]]; then
    cp -r "$hub/$slug-weft/_lym/discussion" "$hub/$slug-weft/_lym/plan" "$scratch/"
  else
    git -C "$weft" archive "$tag" _lym/discussion _lym/plan | tar -x -C "$scratch"
    mv "$scratch/_lym/discussion" "$scratch/_lym/plan" "$scratch/"
  fi
  cards=0
  bytes=0
  for card in "$scratch"/plan/[0-9][0-9]-*.md; do
    case $(basename "$card") in 00-*) continue ;; esac
    cards=$((cards + 1))
    bytes=$((bytes + $(wc -c < "$card")))
  done
  printf '%s\t%s\t%s\t%s\t%s\n' "$slug" "$tag" "$(wc -c < "$scratch/discussion/decision-record.md")" "$cards" "$bytes"
done < "$here/runs.tsv"
