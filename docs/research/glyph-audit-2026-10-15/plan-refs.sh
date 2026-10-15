#!/usr/bin/env bash
# Plan-card reference census for every pf run and every raf run: read the plan cards from the records repo
# at the weft commit that holds the run's webster state.json, count the references under Edit/Create/Move/Delete/Rename/Uses.
# Output: plan-refs.tsv columns: arm, proj, weft_commit, card, label, kind(glyph|planhandle|bare_file|bare_dir), ref
#         plan-runs.tsv columns: arm, proj, weft_commit, ncards (cards in the plan, 00-overview excluded)
set -u
here="$(cd "$(dirname "$0")" && pwd)"
W=/home/knatte/Code/loomyard-LYXHUB/loomyard-weft
: > "$here/plan-runs.tsv"; : > "$here/plan-refs.tsv"; : > "$here/.runs.txt"

# pf runs: one weft commit per project from batches.tsv
awk -F'\t' '$8!="-"{print "pf\t"$2"\t"$8}' "$here/batches.tsv" | sort -u -k2,2 >> "$here/.runs.txt"
# raf runs: one weft commit per session directory, found by the master session id inside state.json
jq -r 'select(.layout=="inline" and .raf)|.sid' "$here/raw.jsonl" | awk -F'/' '{p=$1; sub(/^-home-knatte-Code-loomyard-LYXHUB-/,"",p); print p"\t"$2}' | sort -u | while IFS=$'\t' read -r proj sess; do
  c=$(git -C "$W" log --all -S"$sess" --format=%h -- _lyx/webster/state.json | head -1)
  if [ -n "$c" ]; then printf 'raf\t%s\t%s\n' "$proj" "$c"; else printf 'raf\t%s\t-\n' "$proj"; fi
done >> "$here/.runs.txt"

while IFS=$'\t' read -r arm proj c; do
  if [ "$c" = "-" ]; then printf '%s\t%s\t-\t-\n' "$arm" "$proj" >> "$here/plan-runs.tsv"; continue; fi
  files=$(git -C "$W" ls-tree -r --name-only "$c" -- _lyx/plan | grep -v '/00-overview' )
  n=$(printf '%s\n' "$files" | grep -c . )
  printf '%s\t%s\t%s\t%s\n' "$arm" "$proj" "$c" "$n" >> "$here/plan-runs.tsv"
  for f in $files; do
    card=$(basename "$f" .md)
    git -C "$W" show "$c:$f" | awk -v arm="$arm" -v proj="$proj" -v c="$c" -v card="$card" '
      /^\*\*[A-Za-z]+:\*\*/ { lab = $0; sub(/^\*\*/, "", lab); sub(/:\*\*.*/, "", lab); next }
      /^- `/ {
        if (lab !~ /^(Edit|Create|Move|Delete|Rename|Uses)$/) next
        r = $0; sub(/^- `/, "", r); sub(/`.*/, "", r)
        if (r ~ /^plan:/) k = "planhandle"
        else if (r ~ /#/) k = "glyph"
        else { s = r; sub(/.*\//, "", s); if (r ~ /\/$/ || s !~ /\./) k = "bare_dir"; else k = "bare_file" }
        print arm "\t" proj "\t" c "\t" card "\t" lab "\t" k "\t" r
      }' >> "$here/plan-refs.tsv"
  done
done < "$here/.runs.txt"
cat "$here/plan-runs.tsv"
