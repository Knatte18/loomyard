#!/usr/bin/env bash
# Usage: breaches.sh <dir-with-turns-and-calls> > breaches.tsv
# One row per session from turns.tsv and calls.tsv (extract-turns.sh of the turn audit): the turns that make the batching hook's breaches.
# Columns: slug session kind role turns ctx_total over_turns over_ctx lf_turns lf_ctx l3_turns l3_ctx cf_turns cf_ctx c3_turns c3_ctx ef_turns ef_ctx
# ctx is a turn's input + cache write + cache read; "over" is a turn with ctx at or above CONTEXT_TOKENS (default 150000, the hook's context_tokens).
# A turn counts in a run whatever its own context, and counts in a figure only when its own context is at or above the line.
#
# Lookup chains come in two readings of "a message that holds one lookup and nothing else":
#   tool reading (lf, l3): exactly one Read, Grep or Glob call, the tool calls the hook judged lookups before it counted a read-only Bash command as one.
#   class reading (cf, c3): exactly one call the turn audit classes read or search, which takes in a search made with a Bash grep, find or ls,
#   and is the nearer reading of the hook's lookup since it counts a read-only Bash command.
# The f figures count a lookup message above the line that follows a lookup message (the baseline audit's "single-lookup turn following another");
# the 3 figures count one above the line that ends a run of three or more (the hook's lookup_run).
# ef: an edit message above the line that follows an edit message, an edit message holding only Edit and Write calls
# (the baseline audit's "edits spread over consecutive turns", and the hook's edit_run of 2).
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' -v line="${CONTEXT_TOKENS:-150000}" '
function flush() {
  if (cur == "") return
  print slug, session, kind, role, turns, ctxtotal, overt, overc, lft, lfc, l3t, l3c, cft, cfc, c3t, c3c, eft, efc
}
FNR == 1 { next }
FILENAME == ARGV[1] {
  # calls.tsv: slug session kind role turn call ts tool class key
  id = $1 SUBSEP $2 SUBSEP $5
  if ($8 == "Read" || $8 == "Grep" || $8 == "Glob") lookups[id]++
  if ($9 == "read" || $9 == "search") classed[id]++
  if ($8 == "Edit" || $8 == "Write") edits[id]++
  next
}
{
  # turns.tsv: slug session kind role turn ts ncalls ctx input cache_write cache_read output weight dep
  sid = $1 SUBSEP $2
  if (sid != cur) {
    flush()
    cur = sid; slug = $1; session = $2; kind = $3; role = $4
    turns = ctxtotal = overt = overc = lft = lfc = l3t = l3c = cft = cfc = c3t = c3c = eft = efc = 0
    toolrun = classrun = editrun = 0
  }
  id = $1 SUBSEP $2 SUBSEP $5
  ctx = $8
  turns++; ctxtotal += ctx
  over = (ctx >= line)
  if (over) { overt++; overc += ctx }
  toolrun = ($7 == 1 && lookups[id] == 1) ? toolrun + 1 : 0
  classrun = ($7 == 1 && classed[id] == 1) ? classrun + 1 : 0
  editrun = ($7 >= 1 && edits[id] == $7) ? editrun + 1 : 0
  if (over && toolrun >= 2) { lft++; lfc += ctx }
  if (over && toolrun >= 3) { l3t++; l3c += ctx }
  if (over && classrun >= 2) { cft++; cfc += ctx }
  if (over && classrun >= 3) { c3t++; c3c += ctx }
  if (over && editrun >= 2) { eft++; efc += ctx }
}
END { flush() }
' "$dir/calls.tsv" "$dir/turns.tsv" | awk 'BEGIN { OFS = "\t"; print "slug","session","kind","role","turns","ctx_total","over_turns","over_ctx","lf_turns","lf_ctx","l3_turns","l3_ctx","cf_turns","cf_ctx","c3_turns","c3_ctx","ef_turns","ef_ctx" } { print }'
