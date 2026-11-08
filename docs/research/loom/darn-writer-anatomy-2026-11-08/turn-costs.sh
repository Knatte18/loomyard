#!/usr/bin/env bash
# Usage: turn-costs.sh <dir>
# Reads <dir>/turns.tsv, calls.tsv and prompts.tsv (extract-turns.sh) and writes <dir>/turn-costs.tsv, one row per assistant turn:
#   slug session kind role turn model effort ctx cost_input cost_cache_write cost_cache_read cost_output cost thinking ncalls stop
#   round activity gap_s trigger prev_ctx rewrite rewrite_tokens rewrite_usd
# Prices are API list prices in USD per million tokens, from internal/shuttleengine/claudeengine/pricing.go.
# ctx is input + cache write + cache read of the turn.
# round counts the "Gate findings recorded at" prompts that reached the session before the turn: 0 is the writer's first pass.
# activity, for the darn writer's main session:
#   research        turns before the first turn that holds an Edit or Write call, in round 0
#   gate            a round-0 turn from the first edit on that holds a `lym gate` or `go test` call
#   post-gate-edit  a round-0 turn after the writer's first gate turn that holds an Edit or Write call: a fix of what the gate showed
#   post-gate-other a round-0 turn after the writer's first gate turn without one: reads, commits, the change description
#   reprompt        a turn in a round of 1 or more: the writer working on the findings of a failed verify gate
#   write           every other turn from the first edit on
# and fork (a lookup subagent of the writer), gatefixer (the cold gate fixer session) and gatefixer-fork otherwise.
# gap_s is the seconds from the previous turn's first streamed record to this turn's, the time between two API requests
# plus this turn's first-token latency; empty for a session's first turn.
# trigger is what ended the previous turn: gate (a `lym gate` call), gotest (`go test`), agent, tool (any other call),
# or, when the previous turn made no call, the class of the prompt that arrived: gatefindings, notification, creel, resume, other, none.
# rewrite is 1 when the previous context was 20000 tokens or more, this turn read less than half of it from cache,
# and the context did not shrink to under half (a compaction): the turn paid a cache write for a prefix the previous turn had cached.
# rewrite_tokens is the previous context less this turn's cache read (at most the shorter of the two contexts), and rewrite_usd prices
# those tokens at the model's cache-write rate less its cache-read rate, the extra the rewrite cost over a warm read.
# Needs bash, sort and awk; reads only, apart from turn-costs.tsv.
set -euo pipefail
dir=$1
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

# Event stream: calls first (type C), then per session turns (T) and prompts (P) in time order.
awk -F'\t' -v OFS='\t' '
FILENAME == ARGV[1] && FNR > 1 { print $2, $6, 1, $0; next }
FILENAME == ARGV[2] && FNR > 1 { print $2, $5, 0, $0 }
' "$dir/turns.tsv" "$dir/prompts.tsv" | sort -t$'\t' -k1,1 -k2,2n -k3,3n > "$tmp"

awk -F'\t' -v OFS='\t' -v callsfile="$dir/calls.tsv" '
function price(model, which) {
  if (model in pin) return which == "in" ? pin[model] : which == "cw5" ? pcw5[model] : which == "cw1" ? pcw1[model] : which == "cr" ? pcr[model] : pout[model]
  return 0
}
function promptclass(head) {
  if (head ~ /^Gate findings recorded at/) return "gatefindings"
  if (head ~ /^<task-notification>/ && head ~ /creel mail/) return "creel"
  if (head ~ /^<task-notification>/) return "notification"
  if (head ~ /^\[lym\]/) return "resume"
  return "other"
}
BEGIN {
  pin["claude-opus-5-5"] = 4;   pcw5["claude-opus-5-5"] = 5;     pcw1["claude-opus-5-5"] = 8;  pcr["claude-opus-5-5"] = 0.20; pout["claude-opus-5-5"] = 20
  pin["claude-opus-5"] = 5;     pcw5["claude-opus-5"] = 6.25;    pcw1["claude-opus-5"] = 10;   pcr["claude-opus-5"] = 0.50;   pout["claude-opus-5"] = 25
  pin["claude-sonnet-5-5"] = 2; pcw5["claude-sonnet-5-5"] = 2.5; pcw1["claude-sonnet-5-5"] = 4; pcr["claude-sonnet-5-5"] = 0.20; pout["claude-sonnet-5-5"] = 10
  pin["claude-sonnet-5"] = 2;   pcw5["claude-sonnet-5"] = 2.5;   pcw1["claude-sonnet-5"] = 4;   pcr["claude-sonnet-5"] = 0.20;  pout["claude-sonnet-5"] = 10
  pin["claude-fable-5-1"] = 10; pcw5["claude-fable-5-1"] = 12.5; pcw1["claude-fable-5-1"] = 20; pcr["claude-fable-5-1"] = 0.25; pout["claude-fable-5-1"] = 50
  pin["claude-haiku-5-5"] = 0.10; pcw5["claude-haiku-5-5"] = 0.125; pcw1["claude-haiku-5-5"] = 0.20; pcr["claude-haiku-5-5"] = 0.01; pout["claude-haiku-5-5"] = 0.50
  while ((getline line < callsfile) > 0) {
    n = split(line, f, "\t")
    if (f[1] == "slug") continue
    id = f[2] SUBSEP f[5]
    if (f[7] == "Edit" || f[7] == "Write" || f[7] == "MultiEdit") hasedit[id] = 1
    if (f[8] == "gate" || f[13] ~ /(^|[ ;&|])go test/) { if (f[8] == "gate") hasgate[id] = 1; else hasgotest[id] = 1 }
    if (f[7] == "Agent" || f[7] == "Task") hasagent[id] = 1
    if (f[7] == "Edit" || f[7] == "Write" || f[7] == "MultiEdit") { s = f[2]; if (!(s in firstedit) || f[5] + 0 < firstedit[s]) firstedit[s] = f[5] + 0 }
  }
}
{
  session = $1
  if (session != cur) { cur = session; prev = 0; pending = ""; round = 0; seengate = 0 }
  if ($3 == 0) {
    # P row: $4 slug, $5 session, $6 kind, $7 role, $8 ts, $9 chars, $10 head.
    cls = promptclass($10)
    if (prev) pending = cls
    if (cls == "gatefindings") round++
    next
  }
  # T row: $4 slug, $5 session, $6 kind, $7 role, $8 turn, $9 ts_first, $10 ts_last, $11 model, $12 effort, $13 input,
  # $14 cache_write_5m, $15 cache_write_1h, $16 cache_read, $17 output, $18 thinking, $19 ncalls, $20 stop.
  slug = $4; kind = $6; role = $7; turn = $8; first = $9; last = $10; model = $11; effort = $12
  in_t = $13; cw5 = $14; cw1 = $15; cr = $16; out = $17
  ctx = in_t + cw5 + cw1 + cr
  ci = in_t * price(model, "in") / 1e6; cw = (cw5 * price(model, "cw5") + cw1 * price(model, "cw1")) / 1e6
  cc = cr * price(model, "cr") / 1e6; co = out * price(model, "out") / 1e6
  id = session SUBSEP turn
  activity = kind == "fork" ? (role ~ /^gate-fixer/ ? "gatefixer-fork" : "fork") : (role == "gate-fixer" ? "gatefixer" : "")
  if (activity == "") {
    if (round > 0) activity = "reprompt"
    else if (!(session in firstedit) || turn + 0 < firstedit[session]) activity = "research"
    else if ((id in hasgate) || (id in hasgotest)) { activity = "gate"; seengate = 1 }
    else if (seengate) activity = (id in hasedit) ? "post-gate-edit" : "post-gate-other"
    else activity = "write"
  }
  gap = prev ? first - pfirst : ""
  trigger = ""; rewrite = 0; rt = 0; ru = 0
  if (prev) {
    if (pid in hasgate) trigger = "gate"
    else if (pid in hasgotest) trigger = "gotest"
    else if (pid in hasagent) trigger = "agent"
    else if (pcalls > 0) trigger = "tool"
    else trigger = pending == "" ? "none" : pending
    if (pctx >= 20000 && cr < 0.5 * pctx && ctx >= 0.5 * pctx) {
      rewrite = 1
      rt = (ctx < pctx ? ctx : pctx) - cr
      if (rt < 0) rt = 0
      ru = rt * (price(model, "cw5") - price(model, "cr")) / 1e6
    }
  }
  printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%.5f\t%.5f\t%.5f\t%.5f\t%.5f\t%d\t%d\t%s\t%d\t%s\t%s\t%s\t%s\t%d\t%d\t%.5f\n", \
    slug, session, kind, role, turn, model, effort, ctx, ci, cw, cc, co, ci + cw + cc + co, $18, $19, $20, round, activity, \
    (gap == "" ? "" : sprintf("%.1f", gap)), trigger, (prev ? pctx : ""), rewrite, rt, ru
  prev = 1; pid = id; pfirst = first; pctx = ctx; pcalls = $19; pending = ""
}' "$tmp" > "$dir/turn-costs.tsv.body"
{
  printf 'slug\tsession\tkind\trole\tturn\tmodel\teffort\tctx\tcost_input\tcost_cache_write\tcost_cache_read\tcost_output\tcost\tthinking\tncalls\tstop\tround\tactivity\tgap_s\ttrigger\tprev_ctx\trewrite\trewrite_tokens\trewrite_usd\n'
  cat "$dir/turn-costs.tsv.body"
} > "$dir/turn-costs.tsv"
rm -f "$dir/turn-costs.tsv.body"
