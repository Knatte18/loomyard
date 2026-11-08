#!/usr/bin/env bash
# Usage: summary.sh <dir>
# Prints the tables of report.md as markdown to standard output, from the tables extract-turns.sh, turn-costs.sh, gate-rounds.sh and
# per-darn.sh wrote to <dir>. Every dollar figure is API list price of the tokens in the transcripts.
# Needs bash and gawk (mktime); reads only.
set -euo pipefail
dir=$1
tc=$dir/turn-costs.tsv
pd=$dir/per-darn.tsv
runs=$(tail -n +2 "$pd" | wc -l)

median() { sort -n | awk '{ a[NR] = $1 } END { if (NR) printf "%.2f", (NR % 2 ? a[(NR + 1) / 2] : (a[NR / 2] + a[NR / 2 + 1]) / 2) }'; }

echo "## Sample"
awk -F'\t' 'NR > 1 { n[$4 " " $3]++ } END { for (k in n) print n[k], k }' "$dir/sessions.tsv" | sort -k2
echo "runs listed: $(wc -l < "$dir/slugs.txt")"
echo "runs with a writer transcript: $runs"
echo "runs with more than one writer session: $(awk -F'\t' 'NR > 1 && $4 > 1' "$pd" | wc -l)"
echo "turns: $(awk -F'\t' 'NR > 1 { n[$3 " " $4]++ } END { for (k in n) printf "%s=%d ", k, n[k] }' "$tc")"
echo "first writer session start: $(awk -F'\t' 'NR > 1 && $4 == "main" && $3 == "darn" { print $5 }' "$dir/sessions.tsv" | sort | head -1)"
echo "last writer session start: $(awk -F'\t' 'NR > 1 && $4 == "main" && $3 == "darn" { print $5 }' "$dir/sessions.tsv" | sort | tail -1)"

echo
echo "## Cost by token kind"
echo "| part | turns | fresh input | cache writes | cache reads | output | total |"
echo "| --- | --- | --- | --- | --- | --- | --- |"
awk -F'\t' '
NR > 1 {
  g = ($3 == "main" && $4 == "darn") ? "writer" : ($3 == "fork" ? "forks" : "gate fixer")
  i[g] += $9; w[g] += $10; r[g] += $11; o[g] += $12; t[g] += $13; n[g]++
  I += $9; W += $10; R += $11; O += $12; T += $13
}
END {
  split("writer|forks|gate fixer", order, "|")
  for (k = 1; k <= 3; k++) { g = order[k]; printf "| %s | %d | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f |\n", g, n[g], i[g], w[g], r[g], o[g], t[g] }
  printf "| all | %d | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f |\n", n["writer"] + n["forks"] + n["gate fixer"], I, W, R, O, T
  printf "| share of all | | %.1f%% | %.1f%% | %.1f%% | %.1f%% | 100%% |\n", 100 * I / T, 100 * W / T, 100 * R / T, 100 * O / T
  printf "| share of the writer | | %.1f%% | %.1f%% | %.1f%% | %.1f%% | 100%% |\n", 100 * i["writer"] / t["writer"], 100 * w["writer"] / t["writer"], 100 * r["writer"] / t["writer"], 100 * o["writer"] / t["writer"]
}' "$tc"

echo
echo "## Thinking share of the writer's output"
awk -F'\t' '
# A turn with a thinking block is compared with the turns of the same call count (0, 1, 2, 3 or more) that have none.
NR > 1 && $3 == "main" && $4 == "darn" {
  b = $15 > 3 ? 3 : $15
  if ($14 > 0) { tn[b]++; tc[b] += $12 } else { nn[b]++; nc[b] += $12 }
  rows[++m] = $14 SUBSEP b SUBSEP $12
}
END {
  for (x = 1; x <= m; x++) {
    split(rows[x], f, SUBSEP)
    if (f[1] > 0) { base = nc[f[2]] / nn[f[2]]; ex = f[3] - base; if (ex < 0) ex = 0; est += ex; thinkout += f[3]; thinkturns++ } else plainout += f[3]
  }
  printf "turns with a thinking block %d of %d; their output cost $%.2f, the output of turns without $%.2f\n", thinkturns, m, thinkout, plainout
  printf "estimated thinking, the output above the same-call-count baseline of turns without a block: $%.2f, %.0f%% of the writer output\n", est, 100 * est / (thinkout + plainout)
  for (b = 0; b <= 3; b++) printf "calls %s%d: %d turns with a block, mean output $%.4f; %d turns without, mean output $%.4f\n", (b == 3 ? ">=" : ""), b, tn[b], tc[b] / tn[b], nn[b], nc[b] / nn[b]
}' "$tc"

echo
echo "## Cost by activity"
echo "| activity | turns | cost | share of the writer | share of all | cost per darn |"
echo "| --- | --- | --- | --- | --- | --- |"
awk -F'\t' -v runs="$runs" '
NR > 1 {
  a = ($3 == "main" && $4 == "darn") ? $18 : ($3 == "fork" ? "forks (lookup subagents)" : "gate fixer")
  n[a]++; c[a] += $13; T += $13
  if ($3 == "main" && $4 == "darn") W += $13
}
END {
  split("research|write|gate|post-gate-edit|post-gate-other|reprompt|forks (lookup subagents)|gate fixer", order, "|")
  for (k = 1; k <= 8; k++) {
    a = order[k]
    printf "| %s | %d | $%.2f | %s | %.1f%% | $%.2f |\n", a, n[a], c[a], (k <= 6 ? sprintf("%.1f%%", 100 * c[a] / W) : ""), 100 * c[a] / T, c[a] / runs
  }
  printf "| writer main sessions | | $%.2f | 100%% | %.1f%% | $%.2f |\n", W, 100 * W / T, W / runs
  printf "| all | | $%.2f | | 100%% | $%.2f |\n", T, T / runs
}' "$tc"

echo
echo "## Writer cost by activity and token kind"
echo "| activity | cache writes | cache reads | output | total |"
echo "| --- | --- | --- | --- | --- |"
awk -F'\t' '
NR > 1 && $3 == "main" && $4 == "darn" { w[$18] += $10; r[$18] += $11; o[$18] += $12; t[$18] += $13 }
END {
  split("research|write|gate|post-gate-edit|post-gate-other|reprompt", order, "|")
  for (k = 1; k <= 6; k++) { a = order[k]; printf "| %s | $%.2f | $%.2f | $%.2f | $%.2f |\n", a, w[a], r[a], o[a], t[a] }
}' "$tc"

echo
echo "## Turns before the first edit, per writer session"
awk -F'\t' 'NR > 1 && $3 == "main" && $4 == "darn" { s[$2] = 1; if ($18 == "research") { n[$2]++; c[$2] += $13 } } END { for (x in s) { ts += n[x]; cs += c[x]; m++ } printf "sessions %d, mean %.1f research turns costing $%.2f\n", m, ts / m, cs / m }' "$tc"
echo "first turn of a writer session, mean cost: $(awk -F'\t' 'NR > 1 && $3 == "main" && $4 == "darn" && $5 == 1 { s += $13; n++ } END { printf "$%.3f over %d sessions", s / n, n }' "$tc")"

echo
echo "## Post-gate turns without an edit, before and after the writer's last gate or fix turn"
awk -F'\t' '
NR == FNR { if (FNR > 1 && $3 == "main" && $4 == "darn" && ($18 == "gate" || $18 == "post-gate-edit") && $5 + 0 > last[$2]) last[$2] = $5 + 0; next }
FNR > 1 && $3 == "main" && $4 == "darn" && $18 == "post-gate-other" {
  if ($5 + 0 < last[$2]) { bn++; bc += $13 } else { an++; ac += $13 }
}
END { printf "before the last gate or fix turn: %d turns, $%.2f; after it: %d turns, $%.2f\n", bn, bc, an, ac }' "$tc" "$tc"

echo
echo "## Cache lifetime: turns by the gap since the previous turn, previous context 20000 tokens or more, no Gate-findings prompt"
echo "| gap since the previous turn | turns | rewrote the cache | extra cost of rewrites |"
echo "| --- | --- | --- | --- |"
awk -F'\t' '
NR > 1 && $19 != "" && $21 != "" && $21 >= 20000 && $20 != "gatefindings" {
  g = $19
  b = (g < 120 ? "a under 2 min" : g < 240 ? "b 2 to 4 min" : g < 300 ? "c 4 to 5 min" : g < 330 ? "d 5 to 5.5 min" : g < 600 ? "e 5.5 to 10 min" : g < 3600 ? "f 10 to 60 min" : "g over 60 min")
  n[b]++; r[b] += $22; u[b] += $24
}
END { for (k in n) printf "| %s | %d | %d | $%.2f |\n", substr(k, 3), n[k], r[k], u[k] }' "$tc" | sort
echo "one-hour cache writes in all turns: $(awk -F'\t' 'NR > 1 { s += $12 } END { print s + 0 }' "$dir/turns.tsv") tokens"

echo
echo "## Cache rewrites by what ended the previous turn"
echo "| part | what ended the previous turn | rewrites | tokens rewritten | extra cost |"
echo "| --- | --- | --- | --- | --- |"
awk -F'\t' '
NR > 1 && $22 == 1 {
  g = ($3 == "main" && $4 == "darn") ? "writer" : ($3 == "fork" ? "forks" : "gate fixer")
  k = g " | " $20
  n[k]++; t[k] += $23; u[k] += $24
}
END { for (k in n) printf "| %s | %d | %.1fM | $%.2f |\n", k, n[k], t[k] / 1e6, u[k] }' "$tc" | sort
awk -F'\t' 'NR > 1 { T += $13 } NR > 1 && $22 == 1 { n++; u += $24 } END { printf "| all | | %d | | $%.2f |\nall turns cost $%.2f, so the rewrites are %.1f%% of it\n", n, u, T, 100 * u / T }' "$tc"

echo
echo "## Gate calls by caller"
echo "| caller | with --wait | calls | mean s | longest s | under 30 s | 30 to 60 s | 1 to 2 min | 2 to 4 min | 4 to 5 min | over 5 min | capped | failed | next turn rewrote the cache | extra cost |"
echo "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
awk -F'\t' -v OFS='\t' '
FILENAME == ARGV[1] && FNR > 1 { rw[$2, $5] = $22; ru[$2, $5] = $24; next }
FNR > 1 && $3 == "main" && $8 == "gate" && $9 != "" {
  k = ($4 == "darn" ? "writer" : "gate fixer") " | " ($12 == 1 ? "yes" : "no")
  nx = $5 + 1
  n[k]++; d[k] += $9; if ($9 > m[k]) m[k] = $9
  if ($10 == 1) cap[k]++
  if ($11 == 1) fail[k]++
  if (($2, nx) in rw && rw[$2, nx] == 1) { rwn[k]++; rwu[k] += ru[$2, nx] }
  if ($9 < 30) b1[k]++; else if ($9 < 60) b2[k]++; else if ($9 < 120) b3[k]++; else if ($9 < 240) b4[k]++; else if ($9 < 300) b5[k]++; else b6[k]++
}
END { for (k in n) printf "| %s | %d | %.0f | %.0f | %d | %d | %d | %d | %d | %d | %d | %d | %d | $%.2f |\n", k, n[k], d[k] / n[k], m[k], b1[k], b2[k], b3[k], b4[k], b5[k], b6[k], cap[k], fail[k], rwn[k], rwu[k] }' "$tc" "$dir/calls.tsv" | sort
echo "capped calls end at: $(awk -F'\t' 'NR > 1 && $8 == "gate" && $10 == 1 { printf "%.0f\n", $9 }' "$dir/calls.tsv" | sort -n | uniq -c | tr '\n' ';')"

echo
echo "## The writer's own gate calls by period"
echo "| writer started | runs | runs with a gate call | gate calls | mean calls a run |"
echo "| --- | --- | --- | --- | --- |"
TZ=UTC awk -F'\t' '
function ep(s) { gsub(/[-T:Z]/, " ", s); sub(/\.[0-9]+/, "", s); return mktime(s) }
FILENAME == ARGV[1] { if (FNR > 1 && $3 == "darn" && $4 == "main" && (!($1 in st) || ep($5) < st[$1])) st[$1] = ep($5); next }
FNR > 1 {
  d = strftime("%m-%d", st[$1])
  b = d <= "10-31" ? "a 10-27 to 10-31" : d <= "11-03" ? "b 11-01 to 11-03" : d <= "11-05" ? "c 11-04 to 11-05" : "d 11-06 to 11-08"
  n[b]++; if ($19 > 0) w[b]++; c[b] += $19
}
END { for (b in n) printf "| %s | %d | %d | %d | %.1f |\n", substr(b, 3), n[b], w[b], c[b], c[b] / n[b] }' "$dir/sessions.tsv" "$pd" | sort

echo
echo "## Verify-gate rounds and the cold gate fixer, by whether a fixer existed when the writer started"
echo "| writer started | runs | runs with a fixer session | fixer sessions | cleared the gate | left findings | runs with a Gate-findings prompt | prompts | writer cost in re-prompt rounds | fixer cost |"
echo "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
first_fixer=$(awk -F'\t' 'NR > 1 { print $3 }' "$dir/fixer-sessions.tsv" | sort -n | head -1)
TZ=UTC awk -F'\t' -v cut="$first_fixer" '
function ep(s) { gsub(/[-T:Z]/, " ", s); sub(/\.[0-9]+/, "", s); return mktime(s) }
FILENAME == ARGV[1] { if (FNR > 1 && $3 == "darn" && $4 == "main" && (!($1 in st) || ep($5) < st[$1])) st[$1] = ep($5); next }
FNR > 1 {
  b = st[$1] < cut ? "a before the first fixer session" : "b from the first fixer session on"
  n[b]++; if ($27 > 0) wf[b]++; fs[b] += $27; fl[b] += $28; if ($26 > 0) wp[b]++; rp[b] += $26; rc[b] += $18; fx[b] += $29
}
END { for (b in n) printf "| %s | %d | %d | %d | %d | %d | %d | %d | $%.2f | $%.2f |\n", substr(b, 3), n[b], wf[b], fs[b], fs[b] - fl[b], fl[b], wp[b], rp[b], rc[b], fx[b] }' "$dir/sessions.tsv" "$pd" | sort
echo "first fixer session starts at epoch $first_fixer"
echo
echo "Fixer sessions that cleared the gate against those that left findings:"
awk -F'\t' 'NR > 1 { k = $9; n[k]++; c[k] += $8; s[k] += $5; t[k] += $6 } END { for (k in n) printf "left_findings=%d: %d sessions, mean $%.2f, mean %.0f s, mean %.1f turns\n", k, n[k], c[k] / n[k], s[k] / n[k], t[k] / n[k] }' "$dir/fixer-sessions.tsv" | sort
echo "fixer sessions per run: $(awk -F'\t' 'NR > 1 && $27 > 0 { n[$27]++ } END { for (k in n) printf "%d session(s): %d runs; ", k, n[k] }' "$pd")"
echo "Gate-findings prompts per run: $(awk -F'\t' 'NR > 1 && $26 > 0 { n[$26]++ } END { for (k in n) printf "%d prompt(s): %d runs; ", k, n[k] }' "$pd")"

echo
echo "## The writer's wait at a Gate-findings prompt"
echo "| prompt | prompts | mean writer idle s | longest s | idle 5 min or more | next turn rewrote the cache | extra cost of rewrites | mean fixer s | writer turns in the round | writer cost in the round |"
echo "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
awk -F'\t' '
NR > 1 {
  k = ($6 == "") ? "no fixer ended within 10 minutes before the prompt" : "a cold fixer ended just before the prompt"
  n[k]++; idle[k] += $5; if ($5 + 0 > mx[k]) mx[k] = $5 + 0
  if ($5 >= 300) over[k]++
  if ($9 == 1) { rw[k]++; ru[k] += $10 }
  rtn[k] += $11; rcc[k] += $12
  if ($6 != "") fsec[k] += $6
}
END { for (k in n) printf "| %s | %d | %.0f | %.0f | %d | %d | $%.2f | %s | %d | $%.2f |\n", k, n[k], idle[k] / n[k], mx[k], over[k], rw[k], ru[k], (k ~ /^a cold/ ? sprintf("%.0f", fsec[k] / n[k]) : ""), rtn[k], rcc[k] }' "$dir/gate-prompts.tsv" | sort
echo "median writer idle at a prompt after a fixer: $(awk -F'\t' 'NR > 1 && $6 != "" { print $5 }' "$dir/gate-prompts.tsv" | median) s"
echo "median fixer run before such a prompt: $(awk -F'\t' 'NR > 1 && $6 != "" { print $6 }' "$dir/gate-prompts.tsv" | median) s"

echo
echo "## Writer model and effort"
echo "| model and effort | runs | mean writer cost | median writer cost | mean turns | mean cost a turn | mean changed lines | runs with a re-prompt | mean forks |"
echo "| --- | --- | --- | --- | --- | --- | --- | --- | --- |"
for key in "claude-opus-5-5 medium" "claude-sonnet-5-5 medium" "claude-fable-5-1 high"; do
  med=$(awk -F'\t' -v k="$key" 'NR > 1 && $2 " " $3 == k { print $7 }' "$pd" | median)
  awk -F'\t' -v k="$key" -v med="$med" '
  NR > 1 && $2 " " $3 == k { n++; c += $7; t += $5; f += $12; if ($6 != "") { ln++; l += $6 } rp += ($26 > 0) }
  END { printf "| %s | %d | $%.2f | $%s | %.0f | $%.3f | %.0f | %d | $%.2f |\n", k, n, c / n, med, t / n, c / t, (ln ? l / ln : 0), rp, f / n }' "$pd"
done
echo
echo "Lookup subagent turns by model and effort:"
awk -F'\t' 'NR > 1 && $3 == "fork" && $4 == "darn+fork" { k = $6 " " $7; n[k]++; c[k] += $13 } END { for (k in n) printf "%s: %d turns, $%.2f\n", k, n[k], c[k] }' "$tc" | sort
echo "Writer turns at an effort other than medium: $(awk -F'\t' 'NR > 1 && $3 == "main" && $4 == "darn" && $7 != "medium" { n[$6 " " $7]++ } END { for (k in n) printf "%s: %d; ", k, n[k] }' "$tc")"
echo "Writer runs by model: $(awk -F'\t' 'NR > 1 { n[$2]++ } END { for (k in n) printf "%s: %d; ", k, n[k] }' "$pd")"
echo "Sonnet and Opus writers on the same size: runs of 100 to 1500 changed lines"
for key in "claude-opus-5-5" "claude-sonnet-5-5"; do
  awk -F'\t' -v k="$key" 'NR > 1 && $2 == k && $6 != "" && $6 >= 100 && $6 <= 1500 { n++; c += $7; t += $5; l += $6 } END { printf "%s: %d runs, mean writer cost $%.2f, mean turns %.0f, mean lines %.0f\n", k, n, c / n, t / n, l / n }' "$pd"
done

echo
echo "## Per-run means over the writer runs"
awk -F'\t' 'NR > 1 { n++; w += $7; f += $12; x += $29; t += $5 } END { printf "runs %d; writer $%.2f, forks $%.2f, gate fixer $%.2f, turns %.1f a run\n", n, w / n, f / n, x / n, t / n }' "$pd"
echo "median writer cost: $(awk -F'\t' 'NR > 1 { print $7 }' "$pd" | median); median writer plus forks: $(awk -F'\t' 'NR > 1 { print $7 + $12 }' "$pd" | median)"
echo
echo "## A writer that ends at a finished change: what it would have saved on these runs"
awk -F'\t' '
NR > 1 {
  gate += $15; pge += $16; rep += $18; fixmean_n += 0
  hadwork = ($26 > 0 || $16 > 0)
  if (hadwork) { w++ }
  if ($26 > 0) p += $26
  wr += $7
}
END { printf "writer cost in gate turns $%.2f, post-gate edit turns $%.2f, re-prompt rounds $%.2f; runs with a re-prompt or a post-gate edit %d; prompts %d\n", gate, pge, rep, w, p }' "$pd"
