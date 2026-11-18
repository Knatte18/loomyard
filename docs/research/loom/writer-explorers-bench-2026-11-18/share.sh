#!/usr/bin/env bash
# Usage: share.sh <out-dir>
# Splits the cost of the Plan-Write and Discussion-Write sessions of the slugs in slugs.txt by who spent it:
# the writer itself, its twill and lens forks (agentType fork), and its explorer and lookup subagents
# (agentType general-purpose or Explore), read from each subagent's meta file.
# Writes <out-dir>/share-rows.tsv, one row per transcript, and <out-dir>/share-totals.tsv, one row per writer role.
# Each transcript is priced at its model's list price in USD per million tokens, as the earlier lookups measure priced it;
# a model with no price counts as zero and its messages are counted in the unpriced column.
# Needs bash, jq and awk; reads only, apart from the two tables.
set -euo pipefail
out=$1
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
projects=${PROJECTS:-$HOME/.claude/projects}
mkdir -p "$out"
printf 'slug\trole\tsession\tkind\tagent_type\tturns\tcost_usd\tunpriced_messages\n' > "$out/share-rows.tsv"

# turns, cost and unpriced message count of one transcript
# shellcheck disable=SC2016
costprog='
def price($m): {"claude-opus-5-5": [4, 5, 8, 0.2, 20], "claude-sonnet-5-5": [2, 2.5, 4, 0.2, 10]}[$m];
[.[] | select(.type == "assistant" and .message.id != null)] | group_by(.message.id)
| map(max_by(.message.usage.output_tokens // 0)) as $t
| [($t | length),
   ([$t[] | .message.model as $m | .message.usage as $u | (price($m) // [0, 0, 0, 0, 0]) as $p
     | (($u.input_tokens // 0) * $p[0] + (($u.cache_creation_input_tokens // 0) - ($u.cache_creation.ephemeral_1h_input_tokens // 0)) * $p[1]
        + ($u.cache_creation.ephemeral_1h_input_tokens // 0) * $p[2] + ($u.cache_read_input_tokens // 0) * $p[3] + ($u.output_tokens // 0) * $p[4]) / 1e6] | add // 0),
   ([$t[] | select(price(.message.model) == null)] | length)] | @tsv'

while read -r slug; do
  dir=$projects/-home-knatte-Code-loomyard-LYMHUB-$slug
  [ -d "$dir" ] || continue
  for f in "$dir"/*.jsonl; do
    [ -e "$f" ] || continue
    title=$(jq -r 'select(.type == "custom-title") | .customTitle' "$f" | tail -1)
    role=${title##*:}; role=${role//\"/}
    [ "$role" = plan ] || [ "$role" = discussion ] || continue
    sid=$(basename "$f" .jsonl)
    IFS=$'\t' read -r turns cost unpriced < <(jq -rs "$costprog" "$f")
    printf '%s\t%s\t%s\twriter\t-\t%s\t%s\t%s\n' "$slug" "$role" "$sid" "$turns" "$cost" "$unpriced" >> "$out/share-rows.tsv"
    for meta in "$dir/$sid"/subagents/*.meta.json; do
      [ -e "$meta" ] || continue
      sub=${meta%.meta.json}.jsonl
      [ -e "$sub" ] || continue
      type=$(jq -r '.agentType // "unknown"' "$meta")
      kind=explorer; [ "$type" = fork ] && kind=fork
      IFS=$'\t' read -r turns cost unpriced < <(jq -rs "$costprog" "$sub")
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$role" "$sid" "$kind" "$type" "$turns" "$cost" "$unpriced" >> "$out/share-rows.tsv"
    done
  done
done < "$here/slugs.txt"

awk -F'\t' -v OFS='\t' '
NR == 1 { next }
{ cost[$2, $4] += $7; total[$2] += $7; n[$2, $4]++; runs[$2, $1] = 1; unpriced[$2] += $8 }
END {
  print "role", "runs", "writer_usd", "fork_usd", "explorer_usd", "explorer_subagents", "total_usd", "explorer_share", "unpriced_messages"
  for (key in runs) { split(key, p, SUBSEP); r[p[1]]++ }
  for (role in total)
    printf "%s\t%d\t%.2f\t%.2f\t%.2f\t%d\t%.2f\t%.3f\t%d\n", role, r[role], cost[role, "writer"], cost[role, "fork"], cost[role, "explorer"], n[role, "explorer"], total[role], cost[role, "explorer"] / total[role], unpriced[role]
}' "$out/share-rows.tsv" > "$out/share-totals.tsv"
