#!/usr/bin/env bash
# Usage: family.sh [dir]
# Prints, as markdown from <dir>/cost-table.tsv, family-slugs.txt and family-pre.tsv (default: this script's directory):
#   the per-run table of the darns filed by the one raddle split, with the cost of the Discussion and split runs before them;
#   the per-run table of the tapestry runs;
#   the family against the tapestry runs per 1000 changed lines, each phase with its share;
#   least-squares fits of cost and wall time on thousands of changed lines, for the darns and the tapestry runs.
# Environment: MAXLINES (default 10000) caps the darns of the fit, EXCLUDE_DARN (default `comment-cleanup`, a one-off run)
# leaves one darn out of it, and EXCLUDE_TAPESTRY (default `cold-gate-fixer`, whose Discussion, Plan and Webster
# transcripts are not on this machine) leaves one tapestry run out.
# Phases are matched across the two shapes:
#   Discussion = the Motif run's Discussion (family) or the tapestry's Discussion segment,
#   Split or Plan = the raddle run (family) or the tapestry's Plan segment,
#   Write = the darns' writers and their lookup forks (family) or the tapestry's Webster segment,
#   Review = Code-Review reviewers, judge, seed and fixers, forks included,
#   Gate fixer, Landing and other = the rest.
# Needs bash and awk; prints to standard output only.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
dir=${1:-$here}
maxlines=${MAXLINES:-10000}
exclude_darn=${EXCLUDE_DARN:-comment-cleanup}
exclude_tapestry=${EXCLUDE_TAPESTRY:-cold-gate-fixer}

awk -F'\t' -v OFS='\t' -v maxlines="$maxlines" -v exclude_darn="$exclude_darn" -v exclude_tapestry="$exclude_tapestry" '
FILENAME ~ /family-slugs/ { fam[$1] = 1; next }
FILENAME ~ /family-pre/ {
  if (FNR == 1) next
  if ($2 == "motif") disc += $4
  if ($2 == "raddle") split_usd += $4
  next
}
FNR == 1 { next }
$2 == "darn" && ($1 in fam) {
  nf++; flines += $6; fwall += $7; fwrite += $11 + $12; freview += $13 + $14 + $15; fgate += $16; frest += $17 + $18; ftotal += $19
  fn[nf] = $1; fl[nf] = $6; fw[nf] = $7; fwr[nf] = $11 + $12; frv[nf] = $13 + $14 + $15; fg[nf] = $16; fr[nf] = $17 + $18; ft[nf] = $19; frd[nf] = $20; fp[nf] = $4
}
$2 == "darn" && $6 > 0 && $6 <= maxlines && $1 != exclude_darn {
  nd++; dx[nd] = $6 / 1000; dy[nd] = $19; dw[nd] = $7
}
$2 == "tapestry" && $6 != "" && $1 != exclude_tapestry {
  nt++; tx[nt] = $6 / 1000; ty[nt] = $19; tn[nt] = $1; tl[nt] = $6; tw[nt] = $7; td[nt] = $8; tp[nt] = $9; tb[nt] = $10; trv[nt] = $13 + $14 + $15; tr[nt] = $16 + $17 + $18 + $11 + $12; tt[nt] = $19
  slines += $6; swall += $7; sdisc += $8; splan += $9; sweb += $10; srev += $13 + $14 + $15; srest += $16 + $17 + $18 + $11 + $12; stotal += $19
}
END {
  print "## family"
  print "| darn | profile | changed lines | wall min | writer and forks | Code-Review | gate fixer | landing and other | total | rounds |"
  print "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
  for (i = 1; i <= nf; i++) printf "| `%s` | %s | %d | %.0f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | %d |\n", fn[i], fp[i], fl[i], fw[i], fwr[i], frv[i], fg[i], fr[i], ft[i], frd[i]
  printf "| sum of the %d darns | | %d | %.0f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | |\n", nf, flines, fwall, fwrite, freview, fgate, frest, ftotal
  printf "| Motif Discussion run before them | | | | | | | | $%.2f | |\n", disc
  printf "| raddle run | | | | | | | | $%.2f | |\n", split_usd
  printf "| family | | %d | | | | | | $%.2f | |\n", flines, ftotal + disc + split_usd
  print ""
  print "## tapestry"
  print "| tapestry | changed lines | wall min | Discussion | Plan | Webster | Code-Review | gate fixer, landing and other | total |"
  print "| --- | --- | --- | --- | --- | --- | --- | --- | --- |"
  for (i = 1; i <= nt; i++) printf "| `%s` | %d | %.0f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f |\n", tn[i], tl[i], tw[i], td[i], tp[i], tb[i], trv[i], tr[i], tt[i]
  printf "| sum of the %d | %d | %.0f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f | $%.2f |\n", nt, slines, swall, sdisc, splan, sweb, srev, srest, stotal
  print ""
  print "## family against tapestry per 1000 changed lines"
  print "| phase | family USD per 1000 lines | family share | tapestry USD per 1000 lines | tapestry share |"
  print "| --- | --- | --- | --- | --- |"
  fall = ftotal + disc + split_usd
  fl1 = flines / 1000; sl1 = slines / 1000
  printf "| Discussion | $%.2f | %.0f%% | $%.2f | %.0f%% |\n", disc / fl1, 100 * disc / fall, sdisc / sl1, 100 * sdisc / stotal
  printf "| Split or Plan | $%.2f | %.0f%% | $%.2f | %.0f%% |\n", split_usd / fl1, 100 * split_usd / fall, splan / sl1, 100 * splan / stotal
  printf "| Write | $%.2f | %.0f%% | $%.2f | %.0f%% |\n", fwrite / fl1, 100 * fwrite / fall, sweb / sl1, 100 * sweb / stotal
  printf "| Code-Review | $%.2f | %.0f%% | $%.2f | %.0f%% |\n", freview / fl1, 100 * freview / fall, srev / sl1, 100 * srev / stotal
  printf "| gate fixer, landing and other | $%.2f | %.0f%% | $%.2f | %.0f%% |\n", (fgate + frest) / fl1, 100 * (fgate + frest) / fall, srest / sl1, 100 * srest / stotal
  printf "| total | $%.2f | 100%% | $%.2f | 100%% |\n", fall / fl1, stotal / sl1
  print ""
  print "## least-squares fits of total cost and wall minutes on thousands of changed lines"
  print "| set | runs | USD fixed | USD per 1000 lines | r | wall min fixed | wall min per 1000 lines | r |"
  print "| --- | --- | --- | --- | --- | --- | --- | --- |"
  fit("darns of 1 to " maxlines " lines", nd, dx, dy, dw)
  fit("tapestry runs", nt, tx, ty, tw)
  print ""
  print "The fits predict, for the family size of " flines " lines:"
  print "| set | USD | wall min |"
  print "| --- | --- | --- |"
  printf "| one tapestry | $%.2f | %.0f |\n", ta + tb2 * fl1, twa + twb * fl1
  printf "| darns, as the fit sums them for 12 runs | $%.2f | %.0f |\n", 12 * da + db * fl1, 12 * dwa + dwb * fl1
}
function fit(name, n, x, y, w,   i, sx, sy, sxx, sxy, syy, b, a, r, sw, sxw, sww, bw, aw, rw) {
  for (i = 1; i <= n; i++) { sx += x[i]; sy += y[i]; sxx += x[i] * x[i]; sxy += x[i] * y[i]; syy += y[i] * y[i]; sw += w[i]; sxw += x[i] * w[i]; sww += w[i] * w[i] }
  b = (n * sxy - sx * sy) / (n * sxx - sx * sx); a = (sy - b * sx) / n
  r = (n * sxy - sx * sy) / sqrt((n * sxx - sx * sx) * (n * syy - sy * sy))
  bw = (n * sxw - sx * sw) / (n * sxx - sx * sx); aw = (sw - bw * sx) / n
  rw = (n * sxw - sx * sw) / sqrt((n * sxx - sx * sx) * (n * sww - sw * sw))
  printf "| %s | %d | $%.2f | $%.2f | %.2f | %.0f | %.0f | %.2f |\n", name, n, a, b, r, aw, bw, rw
  if (name ~ /^darns/) { da = a; db = b; dwa = aw; dwb = bw } else { ta = a; tb2 = b; twa = aw; twb = bw }
}
' "$dir/family-slugs.txt" "$dir/family-pre.tsv" "$dir/cost-table.tsv"
