# turn-tables.awk: prints the tables that rest on the picked runs' session transcripts, as markdown.
# Inputs, in this order: pick.tsv, timeline.tsv, sessions.tsv, turns.tsv, tools.tsv; lib.awk is loaded before it.
# A group is a recipe and a period of pick.tsv: darn before, mid and late, tapestry before and after.
# Times are minutes per run unless a column says seconds; a request is one API call, and its time runs from its start to its last block.

function class_of(c) {
  if (c == "model") return "model"
  if (c == "gate") return "gate run by the row"
  if (c == "tool:gate") return "gate run by an agent"
  if (c ~ /^tool:/) return "other tools"
  if (c == "spawn" || c == "start") return "session start"
  if (c == "notify") return "waiting for a notice"
  if (c == "go") return "rows with no session"
  return "idle around turns"
}
function part(producer, recipe) {
  if (recipe == "darn") {
    if (producer == "Darn-Write" || producer == "Describe" || producer == "Finalize") return producer
    return producer ~ /^Code-/ ? "Code review rows" : "Every other row"
  }
  if (producer ~ /^Discussion-/ || producer == "Motif") return "Discussion"
  if (producer ~ /^Plan-/ || producer ~ /^Parts-/) return "Plan"
  if (producer == "Webster" || producer == "Parent-Align" || producer == "Describe" || producer == "Finalize") return producer
  return producer ~ /^Code-/ ? "Code review rows" : "Every other row"
}
function model_name(m, effort) { sub(/^claude-/, "", m); return m "[" effort "]" }
function fit(k,    n, a11, a22, a12, b1, b2, det, b, c) {
  n = q_n[k]
  a11 = q_xx[k] - q_x[k] * q_x[k] / n; a22 = q_zz[k] - q_z[k] * q_z[k] / n; a12 = q_xz[k] - q_x[k] * q_z[k] / n
  b1 = q_xy[k] - q_x[k] * q_y[k] / n; b2 = q_zy[k] - q_z[k] * q_y[k] / n; det = a11 * a22 - a12 * a12
  b = (b1 * a22 - b2 * a12) / det; c = (b2 * a11 - b1 * a12) / det
  return f2((q_y[k] - b * q_x[k] - c * q_z[k]) / n) " | " f2(b) " | " f2(c) " | " f2((b * b1 + c * b2) / (q_yy[k] - q_y[k] * q_y[k] / n))
}
function hist_median(k,    half, seen, ms) { half = h_n[k] / 2; seen = 0; for (ms = 0; ms <= 60000; ms++) { seen += h[k, ms]; if (seen >= half) return ms } return 60000 }

BEGIN {
  FS = "\t"
  ng = split("darn before|darn mid|darn late|tapestry before|tapestry after", G, "|")
  nc = split("model|gate run by the row|gate run by an agent|other tools|session start|idle around turns|rows with no session|waiting for a notice", C, "|")
}

FILENAME == ARGV[1] { if (FNR > 1) { grp[$1] = $3 " " $6; recipe[$1] = $3; size[$1] = $8; members[$3 " " $6]++; when[$1] = ($6 == "before") ? "before" : "after" } next }

FILENAME == ARGV[2] {
  if (FNR == 1) next
  c = class_of($5); g = grp[$1]
  per_run[$1, c] += $6; whole[$1] += $6
  by_part[g, part($3, recipe[$1]), c] += $6; parts[recipe[$1], part($3, recipe[$1])] = 1
  if ($5 == "spawn") spawn[g] += $6
  if ($5 == "start") startup[g] += $6
  if ($5 == "notify" && $6 > 60) waits[$1 " | " $3] += $6
  next
}

FILENAME == ARGV[3] { if (FNR > 1 && $3 == "main") { mains[grp[$1]]++; if ($4 == "gate-fixer") fixers[grp[$1]]++ } next }

FILENAME == ARGV[4] {
  if (FNR == 1 || $7 == "" || $9 < $7) next
  g = grp[$1]; secs = ($9 - $7) / 1000; context = $10 + $11 + $12
  k = g SUBSEP $4 SUBSEP model_name($5, $6); role_keys[k] = g; r_req[k]++; r_sec[k] += secs; r_out[k] += $13; r_think[k] += $14
  if (!((k, $2) in r_seen)) { r_seen[k, $2] = 1; r_sess[k]++ }
  if ($13 > 0 && secs <= 600) {
    nk = split(model_name($5, $6) SUBSEP "all|" model_name($5, $6) SUBSEP when[$1], F, "|")
    for (i = 1; i <= nk; i++) { k = F[i]; x = $13 / 1000; z = context / 100000; q_n[k]++; q_y[k] += secs; q_x[k] += x; q_z[k] += z; q_xx[k] += x * x; q_zz[k] += z * z; q_xz[k] += x * z; q_xy[k] += x * secs; q_zy[k] += z * secs; q_yy[k] += secs * secs; q_out[k] += $13; q_ctx[k] += context }
  }
  s = $1 SUBSEP $2
  if (s in last_end) {
    gap = ($7 - last_end[s]) / 1000; b = gap < 60 ? "under 1 minute" : gap < 300 ? "1 to 5 minutes" : "over 5 minutes"
    c_all[b]++
    if (context > 60000) { c_big[b]++; if ($11 < context / 2) { c_miss[b]++; c_written[b] += $12; c_secs[b] += secs - $13 / 1000 * (($5 ~ /opus/) ? 8.15 : 5.61) } }
  }
  last_end[s] = $9
  if ($3 == "main" && $4 == "darn" && recipe[$1] == "darn") { w_req[$1]++; w_out[$1] += $13; w_sec[$1] += secs; if (context > w_peak[$1]) w_peak[$1] = context }
  if ($4 == "gate-fixer") { x_req[g]++; x_sec[g] += secs }
  if ($3 == "main" && !(s in first_seen)) { first_seen[s] = 1; add("first" SUBSEP when[$1], context) }
  next
}

FILENAME == ARGV[5] {
  if (FNR == 1 || $6 == "agent") next
  g = grp[$1]; secs = ($8 - $7) / 1000
  t_n[g, $6]++; t_sec[g, $6] += secs; cats[$6] = 1
  name = ($5 == "Bash") ? "Bash, " $6 : $5
  if ($5 == "Read" || $5 == "Edit" || $5 == "Write" || $6 == "shell-read" || $6 == "git") { k = name SUBSEP when[$1]; names[name] = 1; ms = int($8 - $7); if (ms > 60000) ms = 60000; h[k, ms]++; h_n[k]++ }
  if ($3 == "main" && $4 == "darn" && recipe[$1] == "darn") { if ($6 == "read" || $6 == "shell-read" || $6 == "search") { w_reads[$1]++; w_chars[$1] += $9 } if ($6 == "edit") w_edits[$1]++ }
  if ($4 == "gate-fixer" && $6 == "gate") { x_gate[g]++; x_gate_sec[g] += secs }
  if ($4 == "webster+sub" && $6 == "gate") { fork_gate[g]++; fork_gate_sec[g] += secs }
  next
}

END {
  for (slug in grp) {
    g = grp[slug]
    for (i = 1; i <= nc; i++) add(g SUBSEP C[i], per_run[slug, C[i]] / 60)
    add(g SUBSEP "all", whole[slug] / 60); add(g SUBSEP "less notice", (whole[slug] - per_run[slug, "waiting for a notice"]) / 60)
    add(g SUBSEP "lines", size[slug])
  }
  print "## Where a picked run's wall time went, by class: mean (median) minutes per run\n"
  head("Class|darn before|darn mid|darn late|tapestry before|tapestry after")
  line = "| Runs |"; for (j = 1; j <= ng; j++) line = line " " members[G[j]] " |"; print line
  line = "| Changed lines, median |"; for (j = 1; j <= ng; j++) line = line " " f0(med(G[j] SUBSEP "lines")) " |"; print line
  for (i = 1; i <= nc; i++) { line = "| " C[i] " |"; for (j = 1; j <= ng; j++) line = line " " f1(mean(G[j] SUBSEP C[i])) " (" f1(med(G[j] SUBSEP C[i])) ") |"; print line }
  line = "| All rows |"; for (j = 1; j <= ng; j++) line = line " " f1(mean(G[j] SUBSEP "all")) " (" f1(med(G[j] SUBSEP "all")) ") |"; print line
  line = "| All rows less waiting for a notice |"; for (j = 1; j <= ng; j++) line = line " " f1(mean(G[j] SUBSEP "less notice")) " (" f1(med(G[j] SUBSEP "less notice")) ") |"; print line

  for (j = 1; j <= ng; j++) {
    g = G[j]; split(g, w, " ")
    print "\n## " g ": mean minutes per run, by part of the recipe and class\n"
    head("Part|" "model|gate run by the row|gate run by an agent|other tools|session start|idle around turns|rows with no session|waiting for a notice|all")
    np = split("Darn-Write|Discussion|Plan|Webster|Parent-Align|Code review rows|Describe|Finalize|Every other row", P, "|")
    for (a = 1; a <= np; a++) {
      if (!((w[1], P[a]) in parts)) continue
      line = "| " P[a] " |"; all = 0
      for (i = 1; i <= nc; i++) { v = by_part[g, P[a], C[i]] / members[g] / 60; all += v; line = line " " f1(v) " |" }
      print line " " f1(all) " |"
    }
  }

  print "\n## Requests by role and model: per run (roles over 20 seconds of request time per run)\n"
  head("Group|Role|Model|Sessions|Requests|Output tokens, thousands|Thinking share|Request seconds|Seconds per request")
  n = 0; for (k in role_keys) if (r_sec[k] / members[role_keys[k]] >= 20) order[++n] = k
  for (i = 2; i <= n; i++) { t = order[i]; for (j = i - 1; j >= 1 && (role_keys[order[j]] > role_keys[t] || (role_keys[order[j]] == role_keys[t] && r_sec[order[j]] < r_sec[t])); j--) order[j + 1] = order[j]; order[j + 1] = t }
  for (i = 1; i <= n; i++) { k = order[i]; split(k, p, SUBSEP); m = members[p[1]]; print "| " p[1] " | " p[2] " | " p[3] " | " f1(r_sess[k] / m) " | " f1(r_req[k] / m) " | " f1(r_out[k] / m / 1000) " | " pct(r_think[k], r_out[k]) " | " f0(r_sec[k] / m) " | " f1(r_sec[k] / r_req[k]) " |" }

  print "\n## Request time fitted on output tokens and context: seconds = fixed + per 1000 output tokens + per 100000 context tokens\n"
  head("Model and period|Requests|Fixed s|Per 1000 output tokens s|Per 100000 context tokens s|R squared|Mean output tokens|Mean context tokens")
  n = 0; for (k in q_n) if (q_n[k] >= 200) order[++n] = k
  for (i = 2; i <= n; i++) { t = order[i]; for (j = i - 1; j >= 1 && order[j] > t; j--) order[j + 1] = order[j]; order[j + 1] = t }
  for (i = 1; i <= n; i++) { k = order[i]; split(k, p, SUBSEP); print "| " p[1] ", " p[2] " | " q_n[k] " | " fit(k) " | " f0(q_out[k] / q_n[k]) " | " f0(q_ctx[k] / q_n[k]) " |" }

  print "\n## Prompt cache: requests over 60000 context tokens that read under half from cache, by the gap since the session's request before\n"
  head("Gap|Requests|Over 60000 context tokens|Misses|Tokens written again per miss|Seconds over the fit per miss")
  nb = split("under 1 minute|1 to 5 minutes|over 5 minutes", B, "|")
  for (i = 1; i <= nb; i++) { b = B[i]; print "| " b " | " c_all[b] + 0 " | " c_big[b] + 0 " | " c_miss[b] + 0 " | " (c_miss[b] ? f0(c_written[b] / c_miss[b]) : "-") " | " (c_miss[b] ? f1(c_secs[b] / c_miss[b]) : "-") " |" }

  print "\n## Tool calls per run by category: calls; seconds\n"
  head("Category|darn before|darn mid|darn late|tapestry before|tapestry after")
  n = split("gate|lym|git|shell-other|shell-read|read|edit|search|skill|other", K, "|")
  for (i = 1; i <= n; i++) { line = "| " K[i] " |"; for (j = 1; j <= ng; j++) line = line " " f1(t_n[G[j], K[i]] / members[G[j]]) "; " f1(t_sec[G[j], K[i]] / members[G[j]]) " |"; print line }

  print "\n## Tool latency, every picked run: median milliseconds per call\n"
  head("Tool|Calls before|Median before|Calls after|Median after")
  n = split("Read|Edit|Write|Bash, shell-read|Bash, git", K, "|")
  for (i = 1; i <= n; i++) print "| " K[i] " | " h_n[K[i] SUBSEP "before"] + 0 " | " hist_median(K[i] SUBSEP "before") " | " h_n[K[i] SUBSEP "after"] + 0 " | " hist_median(K[i] SUBSEP "after") " |"

  for (slug in w_req) {
    g = grp[slug]
    add(g SUBSEP "w req", w_req[slug]); add(g SUBSEP "w out", w_out[slug] / 1000); add(g SUBSEP "w sec", w_sec[slug]); add(g SUBSEP "w peak", w_peak[slug] / 1000)
    add(g SUBSEP "w reads", w_reads[slug]); add(g SUBSEP "w chars", w_chars[slug] / 1000); add(g SUBSEP "w edits", w_edits[slug])
    add(g SUBSEP "w chars per line", w_chars[slug] / size[slug]); add(g SUBSEP "w out per line", w_out[slug] / size[slug])
  }
  print "\n## The darn writer's own session: median per run\n"
  head("Period|Sessions|Requests|Request seconds|Output tokens, thousands|Peak context, thousands|Read and search calls|Characters read, thousands|Edit calls|Characters read per changed line|Output tokens per changed line")
  for (j = 1; j <= 3; j++) { g = G[j]; print "| " g " | " cnt[g SUBSEP "w req"] " | " f0(med(g SUBSEP "w req")) " | " f0(med(g SUBSEP "w sec")) " | " f1(med(g SUBSEP "w out")) " | " f0(med(g SUBSEP "w peak")) " | " f0(med(g SUBSEP "w reads")) " | " f0(med(g SUBSEP "w chars")) " | " f0(med(g SUBSEP "w edits")) " | " f0(med(g SUBSEP "w chars per line")) " | " f0(med(g SUBSEP "w out per line")) " |" }
  print "\nContext of a main session's first request, median tokens: " f0(med("first" SUBSEP "before")) " before (" cnt["first" SUBSEP "before"] " sessions), " f0(med("first" SUBSEP "after")) " after (" cnt["first" SUBSEP "after"] " sessions)."

  print "\n## Session starts and gate fixers, per run\n"
  head("Group|Main sessions|Wait for a session's first entry, minutes|First entry to task prompt, minutes|Seconds per session|Gate fixer sessions|Gate fixer requests|Gate fixer request seconds|Gate calls by gate fixers|their seconds")
  for (j = 1; j <= ng; j++) { g = G[j]; m = members[g]; print "| " g " | " f1(mains[g] / m) " | " f1(spawn[g] / m / 60) " | " f1(startup[g] / m / 60) " | " f1((spawn[g] + startup[g]) / mains[g]) " | " f1(fixers[g] / m) " | " f1(x_req[g] / m) " | " f0(x_sec[g] / m) " | " f1(x_gate[g] / m) " | " f0(x_gate_sec[g] / m) " |" }
  print "\nGate calls by Webster forks, per run: " f1(fork_gate["tapestry before"] / members["tapestry before"]) " calls and " f0(fork_gate_sec["tapestry before"] / members["tapestry before"]) " seconds before, " f1(fork_gate["tapestry after"] / members["tapestry after"]) " calls and " f0(fork_gate_sec["tapestry after"] / members["tapestry after"]) " seconds after."

  print "\n## Rows in which a session waited over a minute for a notice\n"
  head("Run and row|Minutes")
  for (k in waits) print "| " k " | " f1(waits[k] / 60) " |"
}
