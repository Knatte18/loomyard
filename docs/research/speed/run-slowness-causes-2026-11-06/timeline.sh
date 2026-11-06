#!/usr/bin/env bash
# Usage: timeline.sh [dir] > timeline.tsv
# Prints, for every recipe row of the picked runs, the seconds each class of time filled, by timeline.awk:
# slug idx producer outcome class seconds
# It reads pick.tsv, rows.tsv, sessions.tsv, prompts.tsv, turns.tsv and tools.tsv from dir (default: this directory).
# Needs bash, awk, sort and date; reads only.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
dir=${1:-$here}

printf 'slug\tidx\tproducer\toutcome\tclass\tseconds\n'
awk -f "$here/timeline.awk" "$dir/pick.tsv" "$dir/rows.tsv" "$dir/sessions.tsv" "$dir/prompts.tsv" "$dir/turns.tsv" "$dir/tools.tsv" |
  sort -t$'\t' -k1,1 -k2,2n -k5,5
