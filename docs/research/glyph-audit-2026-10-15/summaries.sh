#!/usr/bin/env bash
# Reshape the raw TSVs into the summaries quoted in report.md. Writes summaries.txt.
set -u
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here" || exit 1
{
echo "== pack-check classes (pooled)"
awk -F'\t' '{print $8}' pack-check.tsv | sort | uniq -c
echo "== pack-check classes per fork (proj, agent, counts)"
cat pack-check-perfork.tsv
echo "== glyph package vs file directory mismatches in the pack (excluding _test external packages)"
awk -F'\t' '{split($4,g,"#"); f=$5; sub(/\/[^\/]*$/,"",f); if (f!=g[1] && g[1] != f"_test") print $2, $4, $5}' pack-check.tsv | wc -l
echo "== unresolved pack entries by reason"
awk -F'\t' '{print $5}' pack-unres.tsv | sed 's/^names a member.*/plan-handle/' | cut -c1-70 | sort | uniq -c | sort -rn
echo "== control: batch 4/7/13 packs against batch 1's tree"
awk -F'\t' '{print substr($1,7,8),$8}' control-check.tsv | sort | uniq -c
echo "== existing files edited with no pack span, per run"
awk -F'\t' '$7=="yes"&&$4>0{f[$2]++; e[$2]+=$4; F++; E+=$4} END{for(p in f) print p, "files="f[p], "edits="e[p]; print "TOTAL files="F, "edits="E}' missed-files.tsv | sort
echo "== same, by extension (files, edits)"
awk -F'\t' '$7=="yes"&&$4>0{x=$3; sub(/.*\./,"",x); t=(x=="go"&&$3~/_test\.go$/)?"go_test":x; n[t]++; e[t]+=$4} END{for(t in n) print t, n[t], e[t]}' missed-files.tsv | sort -k3,3nr
echo "== existing files with no pack span edited most (non-test, non-md)"
awk -F'\t' '$7=="yes" && $4>0 && $3 !~ /_test\.go$|\.md$/' missed-files.tsv | sort -t$'\t' -k4,4nr | head -12 | cut -f2-6
echo "== forks whose pack was not checkable (no reconstructable start tree)"
awk -F'\t' '$8=="notree"{print $2, substr($1,7,8)}' pack-check.tsv | sort | uniq -c
echo "== plan glyph refs against pack entries (pf, sampled cards)"
awk -F'\t' '
FILENAME==ARGV[1] { c=$3; sub(/.*\//,"",c); sub(/\.md$/,"",c); res[$2"|"c"|"$4]=1; samp[$2"|"c]=1; next }
FILENAME==ARGV[2] { c=$3; sub(/.*\//,"",c); sub(/\.md$/,"",c); un[$2"|"c"|"$4]=1; samp[$2"|"c]=1; next }
$1=="pf" && $6=="glyph" { k=$2"|"$4; if (!(k in samp)) next; g=$2"|"$4"|"$7; if (g in res) R++; else if (g in un) U++; else M++ }
$1=="pf" && $6=="planhandle" { if (($2"|"$4) in samp) PH++ }
END { print "glyph refs resolved (in pack with a span)="R+0, "unresolved(non plan:)="U+0, "absent from pack="M+0, "plan: handles in sampled cards="PH+0 }' pack-check.tsv pack-unres.tsv plan-refs.tsv
echo "== bare file refs by arm, label, extension"
awk -F'\t' '$6=="bare_file"{e=$7; sub(/.*\./,"",e); t=($7 ~ /_test\.go$/)?"go_test":e; print $1,$5,t}' plan-refs.tsv | sort | uniq -c | sort -rn | head -14
echo "== bare non-test go files under Edit or Move in pf cards (examples)"
awk -F'\t' '$1=="pf" && $6=="bare_file" && $7 ~ /\.go$/ && $7 !~ /_test\.go$/ && $7 !~ /doc\.go$/ && ($5=="Edit"||$5=="Move")' plan-refs.tsv | awk -F'\t' '{print $2,$4,$5,$7}' | head -20
} > summaries.txt
{
printf 'arm\tproj\tsession\tcard_writes\tq_glyphs\tq_resolve\tq_expand\tq_toc\tq_other\tquarry_total\tgreps\treads\tls_glob\ttotal_tools\tquarry_share_of_quarry_plus_greps\n'
awk -F'\t' '{q=$6+$7+$8+$9+$10; g=$11+$12; r=$13+$14; printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%s\t%s\t%.0f%%\n",$1,$2,substr($3,1,8),$5,$6,$7,$8,$9,$10,q,g,r,$15,$16,(q+g)?q/(q+g)*100:0}' plan-writers.tsv
} > plan-writers-summary.tsv
awk -F'\t' 'NR>1{a=$1; n[a]++; q[a]+=$10; g[a]+=$11; r[a]+=$12; t[a]+=$14; if($10>0)nq[a]++} END{for(a in n) print a, "writers="n[a], "with_any_quarry="nq[a]+0, "quarry="q[a], "greps="g[a], "reads="r[a], "total_tools="t[a]}' plan-writers-summary.tsv >> summaries.txt
cat summaries.txt
