# timeline.awk: splits each recipe row of the picked runs into what filled its wall time, and prints one line per row and class:
# slug idx producer outcome class seconds
# Inputs, in this order: pick.tsv, rows.tsv, sessions.tsv, prompts.tsv, turns.tsv, tools.tsv.
# The row is cut into ticks of a tenth of a second, and each tick takes the one class of highest rank that covers it, so the classes of a row add up to the row:
#   model         an API request of the row's work is in flight, in any session or subagent
#   tool:<kind>   no request is in flight and a tool call is, the kind being the call's category; a call that only waits on a subagent is no tool call
#   start         a session is between its first entry and its task prompt: the client starting, the skill-load turn and the typing of the prompt
#   gate          a gate span of kind gate that the row's Go code runs, with no session at work
#   spawn         nothing is at work and the next thing to happen is a session's first entry
#   reprompt      nothing is at work and the next thing is a prompt typed into a live session, gate findings among them
#   notify        nothing is at work and the next thing is a notice the client gives a session, such as a finished fork
#   client        nothing is at work and the next thing is a request or a tool call of a session already in a turn
#   end           nothing is at work and nothing more happens in the row: the session's end, the row's checks and its status commit
#   go            a row no session worked in, less its gate spans: Go work alone
# Ranks: model, then tool by the order of the kinds below, then start, then gate; the idle classes fill what is left.

BEGIN {
  FS = "\t"; OFS = "\t"; tick = 100
  nk = split("gate lym git shell-other shell-read search read edit skill other", kinds, " ")
  for (i = 1; i <= nk; i++) rank["tool:" kinds[i]] = 100 - i
  rank["model"] = 200; rank["start"] = 50; rank["gate"] = 40
}

# paint marks the ticks of the current row whose midpoint lies in [from, to) with the class, where it outranks what the tick holds.
# An interval shorter than a tick marks one only when it covers a midpoint, so short calls are neither lost nor rounded up as a whole.
function paint(from, to, class,    a, b, i) {
  if (to <= row_from || from >= row_to) return
  a = int(((from < row_from ? row_from : from) - row_from) / tick + 0.5)
  b = int(((to > row_to ? row_to : to) - row_from) / tick + 0.5)
  for (i = a; i < b && i < ticks; i++) if (!(i in state) || rank[class] > rank[state[i]]) state[i] = class
}

FILENAME == ARGV[1] { if (FNR > 1) picked[$1] = 1; next }
FILENAME == ARGV[2] {
  if (FNR == 1 || !($1 in picked)) next
  n = ++rows[$1]
  r_idx[$1, n] = $5; r_prod[$1, n] = $6; r_out[$1, n] = $7; r_from[$1, n] = $17 * 1000; r_to[$1, n] = $18 * 1000; r_spans[$1, n] = $16
  next
}
FILENAME == ARGV[3] { if (FNR > 1 && $3 == "main") { n = ++nmark[$1]; m_at[$1, n] = $5; m_kind[$1, n] = "spawn"; s_first[$1, $2] = $5 } next }
FILENAME == ARGV[4] {
  if (FNR == 1 || $3 != "main") next
  kind = "reprompt"
  if ($8 == "launch") kind = "spawn"
  else if ($8 == "notice") kind = "notify"
  else if ($8 == "task" && !(($1, $2) in s_task)) { s_task[$1, $2] = $5; kind = "start" }
  n = ++nmark[$1]; m_at[$1, n] = $5; m_kind[$1, n] = kind
  next
}
FILENAME == ARGV[5] { if (FNR > 1) { n = ++nint[$1]; i_from[$1, n] = $7; i_to[$1, n] = $9; i_class[$1, n] = "model"; i_sess[$1, n] = $2 } next }
FILENAME == ARGV[6] { if (FNR > 1 && $6 != "agent") { n = ++nint[$1]; i_from[$1, n] = $7; i_to[$1, n] = $8; i_class[$1, n] = "tool:" $6; i_sess[$1, n] = $2 } next }

END {
  for (slug in picked) for (r = 1; r <= rows[slug]; r++) {
    row_from = r_from[slug, r]; row_to = r_to[slug, r]
    ticks = int((row_to - row_from) / tick)
    delete state; delete next_mark; delete total
    if (ticks <= 0) continue
    worked = 0
    for (n = 1; n <= nint[slug]; n++) {
      from = i_from[slug, n]; to = i_to[slug, n]; class = i_class[slug, n]; sess = i_sess[slug, n]
      if (to <= row_from || from >= row_to) continue
      worked = 1
      if ((slug, sess) in s_task && from < s_task[slug, sess]) continue
      paint(from, to, class)
    }
    for (key in s_task) {
      split(key, k, SUBSEP)
      if (k[1] == slug) paint(s_first[slug, k[2]], s_task[slug, k[2]], "start")
    }
    if (r_spans[slug, r] != "-") {
      ns = split(r_spans[slug, r], sp, ";")
      for (s = 1; s <= ns; s++) { split(sp[s], p, "/"); if (p[2] == "gate") paint(p[3], p[4], "gate") }
    }
    for (n = 1; n <= nmark[slug]; n++) {
      at = m_at[slug, n]
      if (at < row_from || at >= row_to) continue
      worked = 1
      i = int((at - row_from) / tick)
      if (!(i in next_mark) || m_kind[slug, n] == "spawn") next_mark[i] = m_kind[slug, n]
    }
    pending = "end"
    for (i = ticks - 1; i >= 0; i--) {
      if (i in next_mark) pending = next_mark[i]
      if (i in state) { total[state[i]]++; if (!(i in next_mark)) pending = "client" }
      else total[worked ? pending : "go"]++
    }
    for (class in total) print slug, r_idx[slug, r], r_prod[slug, r], r_out[slug, r], class, total[class] * tick / 1000
  }
}
