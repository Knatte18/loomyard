#!/usr/bin/env bash
# Usage: summary.sh <breaches.tsv> <recipes.tsv> > breach-summary.tsv
# Sums breaches.sh rows by recipe and group, then adds an "all" row per recipe and an ALL row over every recipe.
# A group is a main session's role, or "forks" for every fork sub-agent of the recipe.
# Columns: recipe group sessions turns ctx_total over_turns over_ctx over_share lf_turns lf_ctx cf_turns cf_ctx cf_share c3_turns c3_ctx ef_turns ef_ctx ef_share
# The shares are over ctx_total: over_share is the context of turns at or above the hook's line, cf_share and ef_share the context of the breach turns.
# recipes.tsv maps a slug to its recipe; a slug it lacks is an error.
set -euo pipefail
awk -F'\t' -v OFS='\t' '
function add(key, recipe, group,    i) {
  if (!(key in seen)) { seen[key] = 1; order[++count] = key; rec[key] = recipe; grp[key] = group }
  n[key]++
  for (i = 5; i <= 18; i++) sum[key, i] += $i
}
function share(a, b) { return b > 0 ? sprintf("%.3f", a / b) : "" }
FILENAME == ARGV[1] { if (FNR > 1) recipe[$1] = $2; next }
FNR == 1 { next }
{
  if (!($1 in recipe)) { print "summary: no recipe for " $1 > "/dev/stderr"; bad = 1; exit 1 }
  g = ($3 == "fork") ? "forks" : $4
  add($1 == "" ? "" : recipe[$1] SUBSEP g, recipe[$1], g)
  add(recipe[$1] SUBSEP "all", recipe[$1], "all")
  add("ALL" SUBSEP "all", "ALL", "all")
}
END {
  if (bad) exit 1
  print "recipe", "group", "sessions", "turns", "ctx_total", "over_turns", "over_ctx", "over_share", "lf_turns", "lf_ctx", "cf_turns", "cf_ctx", "cf_share", "c3_turns", "c3_ctx", "ef_turns", "ef_ctx", "ef_share"
  for (i = 1; i <= count; i++) {
    k = order[i]
    print rec[k], grp[k], n[k], sum[k, 5], sum[k, 6], sum[k, 7], sum[k, 8], share(sum[k, 8], sum[k, 6]), sum[k, 9], sum[k, 10], sum[k, 13], sum[k, 14], share(sum[k, 14], sum[k, 6]), sum[k, 15], sum[k, 16], sum[k, 17], sum[k, 18], share(sum[k, 18], sum[k, 6])
  }
}' "$2" "$1" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k1,1 -k2,2; }
