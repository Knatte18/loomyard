#!/usr/bin/env bash
# Usage: session-metrics.sh <dir-with-turns-and-calls> > sessions.tsv
# One row per session from turns.tsv and calls.tsv (extract-turns.sh):
# slug session kind role turns calls single zero weight
#   m_runs m_turns m_saved_turns m_weight m_saved_weight        (strict: no call names a token out of the previous result)
#   u_runs u_turns u_saved_turns u_weight u_saved_weight        (upper bound: class only)
#   edit_calls edit_turns first_edit_turn reads_after_edit switches reedits gate testlint validate
#   weight_after_first_edit check_pairs edit_pairs edit_pair_weight
# Mergeable classes: read search git-read lym-read quarry skill (gather calls); a gate, test, lint or validate call is never merged,
# since a repeat follows an edit or a busy gate slot. check_pairs counts consecutive single-call turns that both run a check.
# A run is two or more consecutive single-call turns of mergeable classes;
# in the strict count a turn whose call names a path-like token found in the previous turn's results starts a new run.
# saved turns and saved weight leave out the run's first turn, the one the merged turn replaces.
# edit_pairs counts single-call edit turns that follow a single-call edit turn, edit_pair_weight their weight: an upper bound on edits that could share a turn.
set -euo pipefail
dir=$1
awk -F'\t' -v OFS='\t' '
function mergeable(c) { return c ~ /^(read|search|git-read|lym-read|quarry|skill)$/ }
function ischeck(c) { return c ~ /^(gate|test-lint|validate)$/ }
function phase(c) {
  if (c ~ /^(read|search|git-read|lym-read|quarry)$/) return "R"
  if (c ~ /^(edit|edit-script)$/) return "E"
  return ""
}
FNR == 1 { next }
FILENAME == ARGV[1] {
  # calls.tsv: slug session kind role turn call ts tool class key
  sid = $1 SUBSEP $2
  cls[sid, $5, $6] = $9
  key[sid, $5, $6] = $10
  next
}
function close_run(kind,    saved) {
  if (rl[kind] >= 2) {
    runs[kind]++; rturns[kind] += rl[kind]; rsaved[kind] += rl[kind] - 1
    rw[kind] += rsum[kind]; rsw[kind] += rsum[kind] - rfirst[kind]
  }
  rl[kind] = 0; rsum[kind] = 0; rfirst[kind] = 0
}
function add_run(kind, w) {
  if (rl[kind] == 0) rfirst[kind] = w
  rl[kind]++; rsum[kind] += w
}
function flush(   kinds) {
  if (cur == "") return
  close_run("m"); close_run("u")
  print slug, session, kind, role, turns, calls, single, zero, w,
    runs["m"] + 0, rturns["m"] + 0, rsaved["m"] + 0, rw["m"] + 0, rsw["m"] + 0,
    runs["u"] + 0, rturns["u"] + 0, rsaved["u"] + 0, rw["u"] + 0, rsw["u"] + 0,
    editcalls, editturns, (firstedit == 0 ? "" : firstedit), readsafter, switches, reedits, gate, testlint, validate, wafter, cpairs, epairs, epweight
}
{
  # turns.tsv: slug session kind role turn ts ncalls ctx input cache_write cache_read output weight dep
  id = $1 SUBSEP $2
  if (id != cur) {
    flush()
    cur = id; slug = $1; session = $2; kind = $3; role = $4
    turns = calls = single = zero = 0; w = 0
    delete runs; delete rturns; delete rsaved; delete rw; delete rsw; delete rl; delete rsum; delete rfirst
    editcalls = editturns = firstedit = readsafter = switches = reedits = gate = testlint = validate = cpairs = epairs = epweight = 0; wafter = 0; prevcheck = prevedit = 0
    lastphase = ""; delete edited
  }
  t = $5; n = $7; wt = $13; dep = $14
  turns++; calls += n; w += wt
  if (n == 1) single++
  if (n == 0) zero++
  if (firstedit > 0) wafter += wt
  # run tracking
  c1 = (n == 1) ? cls[id, t, 1] : ""
  if (n == 1 && mergeable(c1)) {
    add_run("u", wt)
    if (dep == 1) close_run("m")
    add_run("m", wt)
  } else { close_run("m"); close_run("u") }
  thischeck = (n == 1 && ischeck(cls[id, t, 1]))
  if (thischeck && prevcheck) cpairs++
  prevcheck = thischeck
  thisedit = (n == 1 && phase(cls[id, t, 1]) == "E")
  if (thisedit && prevedit) { epairs++; epweight += wt }
  prevedit = thisedit
  # call-level read/edit/test pattern
  hasedit = 0
  for (j = 1; j <= n; j++) {
    c = cls[id, t, j]; p = phase(c)
    if (c == "gate") gate++
    else if (c == "test-lint") testlint++
    else if (c == "validate") validate++
    if (p == "E") {
      editcalls++; hasedit = 1
      if (firstedit == 0) { firstedit = t; wafter += wt }
      if (c == "edit") { k = key[id, t, j]; if (k in edited) reedits++; edited[k] = 1 }
    }
    if (p == "R" && firstedit > 0 && t > firstedit) readsafter++
    if (p != "") { if (lastphase != "" && p != lastphase) switches++; lastphase = p }
  }
  if (hasedit) editturns++
}
END { flush() }
' "$dir/calls.tsv" "$dir/turns.tsv" | awk 'BEGIN{OFS="\t"; print "slug","session","kind","role","turns","calls","single","zero","weight","m_runs","m_turns","m_saved_turns","m_weight","m_saved_weight","u_runs","u_turns","u_saved_turns","u_weight","u_saved_weight","edit_calls","edit_turns","first_edit_turn","reads_after_edit","switches","reedits","gate","testlint","validate","weight_after_first_edit","check_pairs","edit_pairs","edit_pair_weight"} {print}'
