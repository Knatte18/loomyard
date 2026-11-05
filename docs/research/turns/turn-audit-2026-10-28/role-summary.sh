#!/usr/bin/env bash
# Usage: role-summary.sh <sessions.tsv> > role-summary.tsv
# Sums session-metrics.sh rows per role, then adds an ALL-main and an ALL-fork row.
# Columns: role sessions turns turns_per_session calls calls_per_turn single_share zero_share weight
#   m_saved_turns m_saved_per_session m_saved_weight_share u_saved_turns u_saved_per_session u_saved_weight_share
#   edit_sessions edit_calls edit_turns calls_per_edit_turn reads_after_edit_per_edit_session switches_per_edit_session
#   reedits_per_edit_session gate_per_session testlint_per_session validate_per_session weight_after_edit_share check_pairs_per_session edit_pairs_per_session edit_pair_weight_share
# Per-edit-session figures divide by sessions with at least one edit call; the weight share is over those sessions' weight.
set -euo pipefail
awk -F'\t' -v OFS='\t' '
function add(r, kind) {
  ns[r]++; turns[r] += $5; calls[r] += $6; single[r] += $7; zero[r] += $8; w[r] += $9
  ms[r] += $12; mw[r] += $14; us[r] += $17; uw[r] += $19
  gate[r] += $26; tl[r] += $27; val[r] += $28; cp[r] += $30; ep[r] += $31; epw[r] += $32
  if ($20 > 0) { es[r]++; ec[r] += $20; et[r] += $21; ra[r] += $23; sw[r] += $24; re[r] += $25; ew[r] += $9; wa[r] += $29 }
}
function f(a, b) { return b > 0 ? sprintf("%.2f", a / b) : "" }
NR == 1 { next }
{ add($4); roles[$4] = 1; add($3 == "main" ? "ALL main" : "ALL fork") }
END {
  roles["ALL main"] = 1; roles["ALL fork"] = 1
  print "role","sessions","turns","turns_per_session","calls","calls_per_turn","single_share","zero_share","weight",
    "m_saved_turns","m_saved_per_session","m_saved_weight_share","u_saved_turns","u_saved_per_session","u_saved_weight_share",
    "edit_sessions","edit_calls","edit_turns","calls_per_edit_turn","reads_after_edit_per_edit_session","switches_per_edit_session",
    "reedits_per_edit_session","gate_per_session","testlint_per_session","validate_per_session","weight_after_edit_share","check_pairs_per_session","edit_pairs_per_session","edit_pair_weight_share"
  for (r in roles) print r, ns[r], turns[r], f(turns[r], ns[r]), calls[r], f(calls[r], turns[r]), f(single[r], turns[r]), f(zero[r], turns[r]), int(w[r]),
    ms[r], f(ms[r], ns[r]), f(mw[r], w[r]), us[r], f(us[r], ns[r]), f(uw[r], w[r]),
    es[r] + 0, ec[r] + 0, et[r] + 0, f(ec[r], et[r]), f(ra[r], es[r]), f(sw[r], es[r]), f(re[r], es[r]), f(gate[r], ns[r]), f(tl[r], ns[r]), f(val[r], ns[r]), f(wa[r], ew[r]), f(cp[r], ns[r]), f(ep[r], ns[r]), f(epw[r], w[r])
}' "$1" | { read -r header; printf '%s\n' "$header"; sort -t$'\t' -k3,3nr; }
