# run-tables.awk: prints the tables that cover every landed run, as markdown.
# Inputs, in this order: runs.tsv, rows.tsv, gate-runs.tsv, reviews.tsv, repo-size.tsv; lib.awk is loaded before it.
# analyze.sh passes the commit times that cut the eras (cut_before, binaries, check, unlocked, all UTC) and, in here, the slugs with a transcript directory on this machine.
# A darn code run is a darn run that is no bench or docs run; the era tables take those with no machine sleep.
# An era is set by a run's start: before (before cut_before), between (to binaries), after 1 (to check), after 2 (to unlocked) and now.
# Times are minutes unless a column says seconds.

function era(start) { return start < cut_before ? "before" : start < binaries ? "between" : start < check ? "after 1" : start < unlocked ? "after 2" : "now" }
function group(producer) {
  if (producer == "Darn-Write") return "Darn-Write"
  if (producer == "Code-Bouncer" || producer == "Code-Burler-Review" || producer == "Describe" || producer == "Finalize") return producer
  if (producer == "Code-Burler-Fix") return "Code-Burler-Fix"
  return "every other row"
}
function band(overlap) { return overlap < 0.25 ? "alone" : overlap < 1.25 ? "one other" : "two or more" }
function cells(prefix,    i, line) { line = ""; for (i = 1; i <= ne; i++) line = line " " f1(sec[prefix, E[i]] / runs[E[i]] / 60) " |"; return line }

BEGIN {
  FS = "\t"
  ne = split("before|between|after 1|after 2|now", E, "|")
  n = split(here, h, "\n"); for (i = 1; i <= n; i++) transcript[h[i]] = 1
}

FILENAME == ARGV[1] {
  if (FNR == 1) next
  slugs[$1]++
  if ($3 != "darn" || is_bench($1, $23)) next
  machine[$2] = ($1 in transcript) ? "this" : "other"; started[$2] = $4
  if ($6 != 0) next
  e = era($4); code[$2] = e; slug_era[$1] = e; runs[e]++
  add("active" SUBSEP e, $7 / 60); if ($28 != "-") add("lines" SUBSEP e, $28)
  sec["setup", e] += $12; if ($20 != "-") sec["land", e] += $20
  next
}

FILENAME == ARGV[2] {
  if (FNR == 1 || $3 != "darn") next
  if ($2 in machine) {
    total[$2] += $10
    if ($6 == "Darn-Write") { write_gate[$2] += $11; write_rest[$2] += $10 - $11 }
    if ($6 == "Finalize") finalize[$2] += $10
  }
  if (!($2 in code)) next
  e = code[$2]; g = group($6)
  sec[g, e] += $10 - $11; sec[g " gate", e] += $11; rows[e]++
  if ($7 == "skipped") add("skip" SUBSEP e, $10)
  if ($6 == "Darn-Write") { writes[e]++; if ($13 > 0 || $14 > 0) failed[e]++; rework[e] += $13; verify[e] += $12 }
  if ($6 == "Finalize" && $7 == "done" && slugs[$1] == 1) { fin_from[$1] = $17; fin_to[$1] = $18 }
  next
}

FILENAME == ARGV[3] {
  if (FNR == 1) next
  if ($8 > 1) { m = ++ntier; t_from[m] = $16; t_to[m] = $16 + $8; t_key[m] = $3; t_tier[m] = $4; t_wall[m] = $8; t_guard[m] = ($5 == "guard"); t_iso[m] = $1; t_launched[m] = $9; t_found[m] = $12; t_wait[m] = $7 }
  if (!($2 in slug_era) || slugs[$2] != 1) next
  e = slug_era[$2]; key = ($3 == "-") ? "run by an agent, or with no key" : $3
  if (!($2 in gated)) { gated[$2] = 1; gate_runs[e]++ }
  keys[key] = 1; g_wait[key, e] += $7
  if ($5 != "guard") { g_tiers[key, e]++; if ($6 == "failed") g_failed[key, e]++ }
  g_wall[key, e] += $8
  if ($3 == "finalize" && ($2 in fin_from) && $16 >= fin_from[$2] - 2 && $16 <= fin_to[$2]) {
    if (!($2 in f_first) || $16 < f_first[$2]) f_first[$2] = $16
    if ($16 + $8 > f_last[$2]) f_last[$2] = $16 + $8
    f_wall[$2] += $8; f_wait[$2] += $7
  }
  next
}

FILENAME == ARGV[4] {
  if (FNR > 1 && ($5 == "code" || $5 == "plan" || $5 == "discussion")) {
    k = $3 SUBSEP $5 SUBSEP $6; r_n[k]++; r_found[k] += $7; r_gating[k] += $12; r_blocking[k] += $8
    if ($7 == 0) r_none[k]++
    if ($12 == 0) r_soft[k]++
    r_keys[$3 " | " $5 " | " $6] = k
  }
  next
}

FILENAME == ARGV[5] { if (FNR > 1) { go_lines[$1] = $4; test_funcs[$1] = $7 } next }

END {
  print "## Darn code runs with no machine sleep, by era\n"
  head("Measure|before|between|after 1|after 2|now")
  line = "| Runs |"; for (i = 1; i <= ne; i++) line = line " " runs[E[i]] " |"; print line
  line = "| Active time, mean |"; for (i = 1; i <= ne; i++) line = line " " f1(mean("active" SUBSEP E[i])) " |"; print line
  line = "| Active time, median |"; for (i = 1; i <= ne; i++) line = line " " f1(med("active" SUBSEP E[i])) " |"; print line
  line = "| Changed lines, median |"; for (i = 1; i <= ne; i++) line = line " " f0(med("lines" SUBSEP E[i])) " |"; print line
  print "| Setup, before the first row |" cells("setup")
  print "| Darn-Write, less its gate spans |" cells("Darn-Write")
  print "| Darn-Write, gate spans |" cells("Darn-Write gate")
  print "| Code-Bouncer |" cells("Code-Bouncer")
  print "| Code-Burler-Review |" cells("Code-Burler-Review")
  print "| Code-Burler-Fix, less its gate spans |" cells("Code-Burler-Fix")
  print "| Code-Burler-Fix, gate spans |" cells("Code-Burler-Fix gate")
  print "| Describe |" cells("Describe")
  print "| Finalize |" cells("Finalize")
  print "| Every other row |" cells("every other row")
  print "| Land, after the last row |" cells("land")
  line = "| Rows per run |"; for (i = 1; i <= ne; i++) line = line " " f1(rows[E[i]] / runs[E[i]]) " |"; print line

  print "\n## Gate tier runs per darn code run, by era and gate key (tier runs; wall minutes; slot wait minutes)\n"
  head("Gate key|before|between|after 1|after 2|now")
  line = "| Runs with a gate record |"; for (i = 1; i <= ne; i++) line = line " " gate_runs[E[i]] " |"; print line
  for (key in keys) {
    line = "| " key " |"
    for (i = 1; i <= ne; i++) { e = E[i]; line = line " " f1(g_tiers[key, e] / gate_runs[e]) "; " f1(g_wall[key, e] / gate_runs[e] / 60) "; " f1(g_wait[key, e] / gate_runs[e] / 60) " |"; all_wall[e] += g_wall[key, e]; all_wait[e] += g_wait[key, e]; all_tiers[e] += g_tiers[key, e] }
    print line
  }
  line = "| All keys |"; for (i = 1; i <= ne; i++) { e = E[i]; line = line " " f1(all_tiers[e] / gate_runs[e]) "; " f1(all_wall[e] / gate_runs[e] / 60) "; " f1(all_wait[e] / gate_runs[e] / 60) " |" }; print line
  line = "| Wall plus wait |"; for (i = 1; i <= ne; i++) { e = E[i]; line = line " " f1((all_wall[e] + all_wait[e]) / gate_runs[e] / 60) " |" }; print line

  # A tier run's overlap is the summed time other tier runs of the machine ran inside it, over its own wall time.
  for (i = 1; i <= ntier; i++) {
    over = 0
    for (j = i - 1; j >= 1 && t_from[i] - t_from[j] < 3600; j--) if (t_to[j] > t_from[i]) over += (t_to[j] < t_to[i] ? t_to[j] : t_to[i]) - t_from[i]
    for (j = i + 1; j <= ntier && t_from[j] < t_to[i]; j++) over += (t_to[j] < t_to[i] ? t_to[j] : t_to[i]) - t_from[j]
    b = band(over / t_wall[i]); when = t_iso[i] < binaries ? "before" : "after"
    load_n[era(t_iso[i]), b]++; load_all[era(t_iso[i])]++
    if (t_guard[i]) continue
    k = t_key[i] SUBSEP t_tier[i] SUBSEP when SUBSEP b
    o_n[k]++; o_wall[k] += t_wall[i]
    if (t_key[i] == "finalize" && b == "alone") {
      day = substr(t_iso[i], 1, 10); if (day == substr(binaries, 1, 10)) day = day (when == "before" ? " a" : " b")
      d = day SUBSEP t_tier[i]; days[day] = 1; d_n[d]++; d_wall[d] += t_wall[i]; d_launched[d] += t_launched[i]; d_found[d] += t_found[i]
    }
  }
  print "\n## Gate tier wall seconds by what ran beside it, before and after one test binary per package (mean; n in brackets)\n"
  head("Gate key and tier|before, alone|before, two or more beside|after, alone|after, two or more beside")
  nk = split("darn-write|code-burler-fix|finalize", K, "|")
  for (a = 1; a <= nk; a++) {
    delete pass
    for (tier = 0; tier <= 2; tier++) {
      line = "| " K[a] " tier " tier " |"
      nc = split("before/alone|before/two or more|after/alone|after/two or more", C, "|")
      for (c = 1; c <= nc; c++) { split(C[c], p, "/"); k = K[a] SUBSEP tier SUBSEP p[1] SUBSEP p[2]; w = o_n[k] ? o_wall[k] / o_n[k] : 0; pass[c] += w; line = line " " f1(w) " (" o_n[k] + 0 ") |" }
      print line
    }
    print "| " K[a] ", the three tiers | " f0(pass[1]) " | " f0(pass[2]) " | " f0(pass[3]) " | " f0(pass[4]) " |"
  }

  print "\n## Tier runs by what ran beside them, by era (every key; share of tier runs)\n"
  head("Era|Tier runs|alone|one other|two or more")
  for (i = 1; i <= ne; i++) { e = E[i]; print "| " e " | " load_all[e] " | " pct(load_n[e, "alone"], load_all[e]) " | " pct(load_n[e, "one other"], load_all[e]) " | " pct(load_n[e, "two or more"], load_all[e]) " |" }

  print "\n## The whole-repository gate (key finalize) when it ran alone, by day, beside the tree's size\n"
  head("Day|Tier 0 s (n)|Tier 1 s (n)|Tier 2 s (n)|Three tiers s|Binaries run, tier 0|Tests found, tier 0|Go lines|Test functions")
  nd = 0; for (day in days) order[++nd] = day
  for (i = 2; i <= nd; i++) { t = order[i]; for (j = i - 1; j >= 1 && order[j] > t; j--) order[j + 1] = order[j]; order[j + 1] = t }
  for (i = 1; i <= nd; i++) {
    day = order[i]; line = "| " day " |"; three = 0
    for (tier = 0; tier <= 2; tier++) { d = day SUBSEP tier; w = d_n[d] ? d_wall[d] / d_n[d] : 0; three += w; line = line " " f1(w) " (" d_n[d] + 0 ") |" }
    d = day SUBSEP 0; plain = substr(day, 1, 10)
    print line " " f0(three) " | " (d_n[d] ? f0(d_launched[d] / d_n[d]) : "-") " | " (d_n[d] ? f0(d_found[d] / d_n[d]) : "-") " | " go_lines[plain] " | " test_funcs[plain] " |"
  }

  for (slug in f_first) {
    e = slug_era[slug]; pre = f_first[slug] - fin_from[slug] - f_wait[slug]; if (pre < 0) pre = 0
    add("fin pre" SUBSEP e, pre); add("fin wait" SUBSEP e, f_wait[slug]); add("fin wall" SUBSEP e, f_wall[slug]); add("fin tail" SUBSEP e, fin_to[slug] - f_last[slug]); add("fin row" SUBSEP e, fin_to[slug] - fin_from[slug])
    if (pre > 60) queued[e]++
  }
  print "\n## The Finalize row of a darn code run, split (seconds; mean unless marked)\n"
  head("Era|Runs|Row|Before its gate starts|the same, median|Runs over a minute before the gate|Slot wait|Gate tiers|After the gate")
  for (i = 1; i <= ne; i++) { e = E[i]; print "| " e " | " cnt["fin row" SUBSEP e] + 0 " | " f0(mean("fin row" SUBSEP e)) " | " f0(mean("fin pre" SUBSEP e)) " | " f0(med("fin pre" SUBSEP e)) " | " queued[e] + 0 " | " f0(mean("fin wait" SUBSEP e)) " | " f0(mean("fin wall" SUBSEP e)) " | " f0(mean("fin tail" SUBSEP e)) " |" }

  print "\n## A skipped row of a darn code run: what one hand-over costs (seconds)\n"
  head("Era|Skipped rows|Mean|Median")
  for (i = 1; i <= ne; i++) { e = E[i]; print "| " e " | " cnt["skip" SUBSEP e] + 0 " | " f1(mean("skip" SUBSEP e)) " | " f1(med("skip" SUBSEP e)) " |" }

  print "\n## The Darn-Write row's gate: how often it sent work back (seconds are means over all rows)\n"
  head("Era|Darn-Write rows|Rows with a refused gate|Rework seconds|Verify gate seconds")
  for (i = 1; i <= ne; i++) { e = E[i]; print "| " e " | " writes[e] + 0 " | " failed[e] + 0 " (" pct(failed[e], writes[e]) ") | " f0(writes[e] ? rework[e] / writes[e] : 0) " | " f0(writes[e] ? verify[e] / writes[e] : 0) " |" }

  print "\n## Review rounds of every landed run: what a round found\n"
  head("Recipe|Review|Round|Rounds|Findings per round|Gating per round|Blocking per round|Rounds with no finding|Rounds with no gating finding")
  nr = 0; for (label in r_keys) order[++nr] = label
  for (i = 2; i <= nr; i++) { t = order[i]; for (j = i - 1; j >= 1 && order[j] > t; j--) order[j + 1] = order[j]; order[j + 1] = t }
  for (i = 1; i <= nr; i++) { k = r_keys[order[i]]; print "| " order[i] " | " r_n[k] " | " f1(r_found[k] / r_n[k]) " | " f1(r_gating[k] / r_n[k]) " | " f2(r_blocking[k] / r_n[k]) " | " pct(r_none[k], r_n[k]) " | " pct(r_soft[k], r_n[k]) " |" }

  # A run with no transcript directory here ran on the hub's other machine; its row stamps are that machine's wall clock, its own sleep included.
  for (tag in total) {
    start = started[tag]; where = ""
    if (machine[tag] == "this" && (tag in code)) where = (start < cut_before) ? "this machine, before" : (start >= binaries && start < check) ? "this machine, after 1" : ""
    if (machine[tag] == "other") where = (start < cut_before) ? "other machine, before" : (start >= binaries && start < check) ? "other machine, after 1" : ""
    if (where == "") continue
    add("m total" SUBSEP where, total[tag] / 60); add("m gate" SUBSEP where, write_gate[tag]); add("m rest" SUBSEP where, write_rest[tag]); add("m fin" SUBSEP where, finalize[tag])
  }
  print "\n## Darn code runs by machine: row stamps (medians)\n"
  head("Machine and era|Runs|All rows, minutes|Darn-Write less its gate, seconds|Darn-Write gate spans, seconds|Finalize, seconds")
  nw = split("this machine, before|other machine, before|this machine, after 1|other machine, after 1", W, "|")
  for (i = 1; i <= nw; i++) { w = W[i]; print "| " w " | " cnt["m total" SUBSEP w] + 0 " | " f1(med("m total" SUBSEP w)) " | " f0(med("m rest" SUBSEP w)) " | " f0(med("m gate" SUBSEP w)) " | " f0(med("m fin" SUBSEP w)) " |" }
}
