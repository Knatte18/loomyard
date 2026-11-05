#!/usr/bin/env bash
# Usage: writers.sh <dir-with-turns-and-calls> <out-dir>
# From turns.tsv and calls.tsv (extract-turns.sh of the turn audit), writes for the Plan-Write and Discussion-Write main sessions (roles plan and discussion):
#   <out-dir>/writer-turns.tsv:    one row per turn: slug session role turn ctx cost_usd ncalls kind
#   <out-dir>/writer-sessions.tsv: one row per session, then a TOTAL row
#   <out-dir>/writer-chains.tsv:   one row per chain of exploration turns in a row
# A turn's kind is explore when every call in it is a lookup (class read, search, git-read or lym-read of the turn audit: Read, Grep, Glob, or a read-only Bash lookup),
# delegate when it holds an Agent call, and other otherwise. A chain is two or more explore turns in a row.
# cost_usd prices a turn's input, cache write and cache read at the Opus 5.5 list price (claude-opus-5-5 in internal/shuttleengine/claudeengine/pricing.go;
# every writer session of the sample ran that model and wrote no one-hour cache entry).
# explore_result_tokens is the context the explore turns added, the growth from an explore turn to the next turn;
# explore_result_carry_usd prices carrying it: one cache write at 5 USD per million, then a cache read at 0.2 USD per million in each later turn.
# search_turns, search_cost_usd and search_over_line_usd are the explore turns that hold a call other than the Read tool (a Grep, a Glob or a Bash lookup),
# the lookups a subagent can take whole; the rest of the explore turns read files the writer may need to see itself.
# LINE_TOKENS (default 200000) is the context line the "over" figures count from.
set -euo pipefail
dir=$1
out=$2
mkdir -p "$out"
awk -F'\t' -v OFS='\t' -v line="${LINE_TOKENS:-200000}" -v turnsout="$out/writer-turns.tsv" -v sessout="$out/writer-sessions.tsv" -v chainout="$out/writer-chains.tsv" '
function endchain() {
  if (runlen >= 2) {
    print slug, session, role, runstart, runlen, runcalls, runctxfirst, runctxmax, sprintf("%.4f", runcost), sprintf("%.4f", runfirstcost), sprintf("%.4f", runovercost) >> chainout
    chains++; chainturns += runlen; chaincost += runcost; chainfirst += runfirstcost
    if (runlen >= 3) { chains3++; chain3turns += runlen; chain3cost += runcost; chain3first += runfirstcost }
  }
  runlen = 0
}
function flush(   i, resulttok, carry) {
  if (cur == "") return
  endchain()
  # The context an exploration turn adds first shows in the next turn, which pays its cache write; every turn after that reads it.
  resulttok = carry = 0
  for (i in growth) { resulttok += growth[i]; carry += growth[i] * (5 + 0.2 * (turns - (i + 1))) / 1e6 }
  print slug, session, role, turns, sprintf("%.4f", cost), exturns, sprintf("%.4f", excost), sprintf("%.4f", exover), exoverturns, chains + 0, chainturns + 0, sprintf("%.4f", chaincost), chains3 + 0, chain3turns + 0, sprintf("%.4f", chain3cost), sprintf("%.4f", chain3first), agentturns + 0, maxctx, resulttok, sprintf("%.4f", carry), sturns, sprintf("%.4f", scost), sprintf("%.4f", sover) >> sessout
  T_result += resulttok; T_carry += carry; T_sturns += sturns; T_scost += scost; T_sover += sover
  T_turns += turns; T_cost += cost; T_ex += exturns; T_excost += excost; T_exover += exover; T_exoverturns += exoverturns
  T_chains += chains; T_chainturns += chainturns; T_chaincost += chaincost; T_chains3 += chains3; T_chain3turns += chain3turns; T_chain3cost += chain3cost; T_chain3first += chain3first; T_agent += agentturns
  T_n++
}
FNR == 1 { next }
FILENAME == ARGV[1] {
  # calls.tsv: slug session kind role turn call ts tool class key
  if ($3 != "main" || ($4 != "plan" && $4 != "discussion")) next
  id = $1 SUBSEP $2 SUBSEP $5
  ncls[id]++
  if ($9 !~ /^(read|search|git-read|lym-read)$/) nonlookup[id]++
  if ($9 == "agent") agent[id]++
  if ($8 != "Read") nonread[id]++
  next
}
{
  # turns.tsv: slug session kind role turn ts ncalls ctx input cache_write cache_read output weight dep
  if ($3 != "main" || ($4 != "plan" && $4 != "discussion")) next
  sid = $1 SUBSEP $2
  if (sid != cur) {
    flush()
    cur = sid; slug = $1; session = $2; role = $4
    turns = cost = exturns = excost = exover = exoverturns = chains = chainturns = chaincost = chains3 = chain3turns = chain3cost = chain3first = agentturns = maxctx = 0
    runlen = 0
    prevkind = ""; prevctx = 0
    sturns = scost = sover = 0
    split("", growth)
    if (!headers) {
      print "slug", "session", "role", "turn", "ctx", "cost_usd", "ncalls", "kind" > turnsout
      print "slug", "session", "role", "start_turn", "length", "calls", "ctx_first", "ctx_max", "cost_usd", "first_turn_cost_usd", "cost_over_line_usd" > chainout
      print "slug", "session", "role", "turns", "cost_usd", "explore_turns", "explore_cost_usd", "explore_over_line_usd", "explore_over_line_turns", "chains", "chain_turns", "chain_cost_usd", "chains3", "chain3_turns", "chain3_cost_usd", "chain3_first_turn_cost_usd", "agent_turns", "max_ctx", "explore_result_tokens", "explore_result_carry_usd", "search_turns", "search_cost_usd", "search_over_line_usd" > sessout
      headers = 1
    }
  }
  id = sid SUBSEP $5
  c = ($9 * 4 + $10 * 5 + $11 * 0.2 + $12 * 20) / 1e6
  ctx = $8
  kind = "other"
  if ($7 >= 1 && ncls[id] == $7 && nonlookup[id] == 0) kind = "explore"
  else if (agent[id] > 0) kind = "delegate"
  if (prevkind == "explore" && ctx > prevctx) growth[turns] = ctx - prevctx
  prevkind = kind; prevctx = ctx
  turns++; cost += c
  if (ctx > maxctx) maxctx = ctx
  if (kind == "delegate") agentturns++
  print $1, $2, $4, $5, ctx, sprintf("%.4f", c), $7, kind >> turnsout
  if (kind == "explore") {
    exturns++; excost += c
    if (nonread[id] > 0) { sturns++; scost += c; if (ctx >= line) sover += c }
    if (ctx >= line) { exover += c; exoverturns++ }
    if (runlen == 0) { runstart = $5; runcalls = 0; runcost = 0; runfirstcost = c; runctxfirst = ctx; runctxmax = 0; runovercost = 0 }
    runlen++; runcalls += $7; runcost += c
    if (ctx > runctxmax) runctxmax = ctx
    if (ctx >= line) runovercost += c
  } else endchain()
}
END {
  flush()
  print "TOTAL", T_n " sessions", "", T_turns, sprintf("%.4f", T_cost), T_ex, sprintf("%.4f", T_excost), sprintf("%.4f", T_exover), T_exoverturns, T_chains + 0, T_chainturns + 0, sprintf("%.4f", T_chaincost), T_chains3 + 0, T_chain3turns + 0, sprintf("%.4f", T_chain3cost), sprintf("%.4f", T_chain3first), T_agent + 0, "", T_result + 0, sprintf("%.4f", T_carry), T_sturns + 0, sprintf("%.4f", T_scost), sprintf("%.4f", T_sover) >> sessout
}' "$dir/calls.tsv" "$dir/turns.tsv"
