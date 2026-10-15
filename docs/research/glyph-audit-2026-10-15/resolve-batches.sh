#!/usr/bin/env bash
# For each pf fork (pf-forks.tsv: session, start, proj, agent file), find in the records repo the webster state.json
# that holds the batch whose spawnedAt is 1..6 s before the fork's first transcript line; print batch number, cards, startSha, whether the object exists.
# Output: batches.tsv  columns: agent, proj, start, batch, ncards, startSha, sha_exists, weft_commit, spawnedAt, cards
set -u
here="$(cd "$(dirname "$0")" && pwd)"
W=/home/knatte/Code/loomyard-LYXHUB/loomyard-weft
R=/home/knatte/Code/loomyard-LYXHUB/loomyard
: > "$here/batches.tsv"
while IFS=$'\t' read -r sess start proj agent; do
  base="${start%.*}"; epoch=$(date -u -d "${base}Z" +%s)
  found=""
  for d in 1 2 3 4 5 6; do
    ts=$(date -u -d "@$((epoch-d))" +%Y-%m-%dT%H:%M:%SZ)
    c=$(git -C "$W" log --all -S"$ts" --format=%h -- _lyx/webster/state.json | head -1)
    if [ -n "$c" ]; then
      row=$(git -C "$W" show "$c:_lyx/webster/state.json" | jq -r --arg ts "$ts" '.batches|to_entries[]|select(.value.spawnedAt==$ts)|[.key,(.value.cards|length),.value.startSha,.value.spawnedAt,(.value.cards|join(","))]|@tsv' | head -1)
      if [ -n "$row" ]; then found="$c	$row"; break; fi
    fi
  done
  if [ -z "$found" ]; then printf '%s\t%s\t%s\t-\t-\t-\tno\t-\t-\t-\n' "$agent" "$proj" "$start" >> "$here/batches.tsv"; continue; fi
  IFS=$'\t' read -r c b n sha sp cards <<< "$found"
  ex=$(git -C "$R" cat-file -t "$sha" 2>/dev/null || echo no)
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$agent" "$proj" "$start" "$b" "$n" "$sha" "$ex" "$c" "$sp" "$cards" >> "$here/batches.tsv"
done < "$here/pf-forks.tsv"
cut -f1-9 "$here/batches.tsv"
