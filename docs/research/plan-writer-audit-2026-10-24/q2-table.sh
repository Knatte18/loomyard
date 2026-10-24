#!/usr/bin/env bash
# Builds $OUT/q2-table.tsv from $OUT/reads/*.{summary,read,named} (made by reads.sh).
# "exdocs" drops PATTERN.md, CLAUDE.md, pattern/ and docs/ from the read set (background reading, not code).
# Run from anywhere. Env: OUT=<work dir> (default <repo>/.scratch/plan-writer-audit/).
set -u
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"
OUT="${OUT:-$REPO/.scratch/plan-writer-audit}"
cd "$OUT/reads" || exit 1
out=../q2-table.tsv
echo -e "run\tread\tread_named\tpct_read_named\tnamed\tnamed_read\tpct_named_read\tread_exdocs\tread_exdocs_named\tpct_exdocs" > $out
for f in *.summary; do
  s=${f%.summary}
  read -r _ nr rn nn nrd < $f
  grep -vE '^(PATTERN\.md|CLAUDE\.md|pattern/|docs/)' $s.read > $s.read-exdocs
  ne=$(wc -l < $s.read-exdocs); en=0
  while read -r x; do
    grep -qxF -e "$x" $s.named || awk -v f="$x" 'index(f,$0"/")==1{h=1} END{exit !h}' $s.named && en=$((en+1))
  done < $s.read-exdocs
  awk -v s=$s -v nr=$nr -v rn=$rn -v nn=$nn -v nrd=$nrd -v ne=$ne -v en=$en 'BEGIN{printf "%s\t%d\t%d\t%.0f%%\t%d\t%d\t%.0f%%\t%d\t%d\t%.0f%%\n",s,nr,rn,100*rn/nr,nn,nrd,100*nrd/nn,ne,en,(ne?100*en/ne:0)}' >> $out
done
awk -F'\t' 'NR>1{r+=$2;rn+=$3;n+=$5;nr+=$6;e+=$8;en+=$9;c++;pr+=$3/$2;pn+=$6/$5} END{printf "TOTAL(%d runs)\t%d\t%d\t%.0f%%\t%d\t%d\t%.0f%%\t%d\t%d\t%.0f%%\nMEAN-of-run-shares\t\t\t%.0f%%\t\t\t%.0f%%\n",c,r,rn,100*rn/r,n,nr,100*nr/n,e,en,100*en/e,100*pr/c,100*pn/c}' $out >> $out
