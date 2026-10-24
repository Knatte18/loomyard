#!/usr/bin/env bash
# A7 trial check: Webster runs under batcher profile cautious vs wide, from the weft archive tags.
# Read-only on the repos; writes only under OUT (default <repo>/.scratch/batch-budget-trial/), never beside this script.
# Run from anywhere: docs/research/batch-budget-trial-2026-10-24/compare.sh [FROM-date]
# Env: FORCE=1 re-runs tokencount for slugs already cached; WEFT=<records repo> (default <repo>/../loomyard-weft);
# OUT=<work dir> for the cache, the built tokencount binary and the outputs.
# Switch to wide: 2026-10-22T15:50:17+02:00 (commit 216c22a12 in the records repo, batcher.yaml "active: wide").
# A run's profile is read from its own state.json partition (the profile Webster actually used), not guessed from dates.
# Outputs: runs.tsv (one row per run), forks.tsv (one row per fork with estimate vs measured peak), summary.md.
set -u
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"
WEFT="${WEFT:-$REPO/../loomyard-weft}"
FROM="${1:-2026-10-15}"
OUT="${OUT:-$REPO/.scratch/batch-budget-trial}"; TC="$OUT/tc"; mkdir -p "$TC"
[ -x "$OUT/tokencount-bin" ] || (cd "$REPO" && go build -o "$OUT/tokencount-bin" ./tools/tokencount) || exit 1

mins() { awk -v s="$1" 'BEGIN{h=m=x=0; if(match(s,/[0-9]+h/))h=substr(s,RSTART,RLENGTH-1); if(match(s,/[0-9]+m/))m=substr(s,RSTART,RLENGTH-1); if(match(s,/[0-9]+s/))x=substr(s,RSTART,RLENGTH-1); printf "%.1f",h*60+m+x/60}'; }

: > "$OUT/runs.tsv"; : > "$OUT/forks.tsv"
printf 'slug\ttag_date\tprofile\tcards\tbatches\tcards_per_fork\test_median\tpeak_median\tpeak_max\twebster_cost\twebster_forks_cost\twebster_min\tcost_per_card\trecoveries\tnotdone\tverify_attempts\treview_rounds\treview_findings\tgating\n' > "$OUT/runs.tsv"
printf 'slug\tprofile\tfirst_card\tcards\testimate\tpeak\tpeak_over_est\n' > "$OUT/forks.tsv"

# prefetch: tokencount per slug, 6 in parallel, cached in tc/ (runs without local transcripts leave no file)
git -C "$WEFT" for-each-ref --sort=creatordate --format='%(creatordate:iso8601-strict) %(refname:short)' refs/tags/archive |
awk -v f="$FROM" '$1>=f{print $2}' | while read -r t; do
  git -C "$WEFT" cat-file -e "$t:_lyx/webster/state.json" 2>/dev/null && echo "$t"; done |
xargs -P6 -I{} bash -c 't={}; slug=${t#archive/}; slug=${slug%/*}; md="'"$TC"'/$slug-${t##*/}.md"
  [ -s "$md" ] && [ "'"${FORCE:-}"'" != 1 ] && exit 0
  (cd "'"$REPO"'" && "'"$OUT"'/tokencount-bin" -out "$md" "$slug" >/dev/null 2>&1) || rm -f "$md"'

git -C "$WEFT" for-each-ref --sort=creatordate --format='%(creatordate:iso8601-strict) %(refname:short)' refs/tags/archive |
awk -v f="$FROM" '$1>=f' | while read -r d t; do
  st=$(git -C "$WEFT" show "$t:_lyx/webster/state.json" 2>/dev/null) || continue
  slug=${t#archive/}; slug=${slug%/*}
  prof=$(jq -r '[.partition[]?.profile]|unique|join("+")' <<<"$st")
  cards=$(jq '[.partition[]?.cards|length]|add // 0' <<<"$st")
  batches=$(jq '.partition|length' <<<"$st")
  rec=$(jq '[.batches[]?|(.recoveries//0)]|add // 0' <<<"$st")
  notdone=$(jq '[.batches[]?|select(.status!="done")]|length' <<<"$st")
  files=$(git -C "$WEFT" ls-tree -r --name-only "$t")
  vatt=$(grep -c '^_lyx/webster/reports/verify-gate' <<<"$files")
  rounds=$(grep -cE '^_lyx/reviews/webster/round-[0-9]+-review\.md$' <<<"$files")
  fnd=0; gat=0
  for f in $(grep -E '^_lyx/reviews/webster/round-[0-9]+-facts\.md$' <<<"$files"); do
    r=$(git -C "$WEFT" show "$t:$f" | awk -F'|' '/^\| [0-9]+ \|/{a+=$3; g+=$13} END{print a+0, g+0}')
    fnd=$((fnd+${r% *})); gat=$((gat+${r#* }))
  done
  # tokencount: cost, time, fork peaks (needs the run's transcripts on this machine)
  md="$TC/$slug-${t##*/}.md"
  if [ ! -s "$md" ] || [ "${FORCE:-}" = 1 ]; then (cd "$REPO" && "$OUT/tokencount-bin" -out "$md" "$slug" >/dev/null 2>&1) || rm -f "$md"; fi
  wc_=NA; wf=NA; wm=NA
  if [ -s "$md" ]; then
    wc_=$(awk -F'|' '$2~/\*\*Webster\*\*/{gsub(/[ $]/,"",$3); print $3; exit}' "$md")
    wf=$(awk -F'|' '$2~/forks/ && p{gsub(/[ $]/,"",$3); print $3; exit} $2~/\*\*Webster\*\*/{p=1}' "$md")
    wt=$(awk -F'|' '$2~/\*\*Webster\*\*/{gsub(/ /,"",$5); print $5; exit}' "$md"); wm=$(mins "$wt")
    # fork rows -> forks.tsv, paired with the partition entry whose first card the fork opens with
    awk -F'|' '/^## Webster forks/{p=1;next} /^## /{p=0} p && $2!~/run|---/ && NF>5 {gsub(/^ +| +$/,"",$3); gsub(/ /,"",$6); print $3 "\t" $6}' "$md" |
    while IFS=$'\t' read -r cl pk; do
      fc=${cl%%,*}
      es=$(jq -r --arg c "$fc" '[.partition[]|select(.cards[0]==$c)|.estimate][0] // "NA"' <<<"$st")
      ratio=NA; [ "$es" != NA ] && ratio=$(awk -v p="$pk" -v e="$es" 'BEGIN{printf "%.2f",p/e}')
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$prof" "$fc" "$(tr ',' '\n' <<<"$cl" | wc -l)" "$es" "$pk" "$ratio" >> "$OUT/forks.tsv"
    done
  fi
  esm=$(jq -r '[.partition[].estimate]|sort|.[(length/2|floor)]' <<<"$st")
  pm=NA; px=NA
  if [ -s "$md" ]; then
    read -r pm px < <(awk -F'\t' -v s="$slug" '$1==s && $6!="" {print $6}' "$OUT/forks.tsv" | sort -n | awk '{v[++n]=$1} END{if(!n){print "NA NA";exit} print v[int((n+1)/2)], v[n]}')
  fi
  cpc=NA; [ "$wc_" != NA ] && cpc=$(awk -v c="$wc_" -v n="$cards" 'BEGIN{printf "%.2f",c/n}')
  cpf=$(awk -v c="$cards" -v b="$batches" 'BEGIN{printf "%.2f",c/b}')
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "${d%%T*}" "$prof" "$cards" "$batches" "$cpf" "$esm" "$pm" "$px" "$wc_" "$wf" "$wm" "$cpc" "$rec" "$notdone" "$vatt" "$rounds" "$fnd" "$gat" >> "$OUT/runs.tsv"
done

# summary: medians per group
awk -F'\t' -v OFS='\t' '
function med(a,n,  i,k,t){ if(n==0) return "NA"; for(i=1;i<=n;i++)s[i]=a[i]; for(i=1;i<=n;i++)for(k=i+1;k<=n;k++)if(s[k]+0<s[i]+0){t=s[i];s[i]=s[k];s[k]=t}; return (n%2)?s[(n+1)/2]:(s[n/2]+s[n/2+1])/2 }
function add(g,col,val){ if(val!="NA"&&val!=""){ N[g,col]++; V[g,col,N[g,col]]=val } }
NR>1 { g=($3~/wide/)?"wide":"cautious"; G[g]++; cols="4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19"
  split(cols,cs," "); for(i in cs){c=cs[i]; add(g,c,$c); if(g=="cautious" && $4>=10) add("cautious>=10cards",c,$c)}
  if(g=="cautious" && $4>=10) G["cautious>=10cards"]++ }
END{
  split("cards batches cards/fork est_median_peak measured_median_peak measured_max_peak webster_cost$ webster_forks_cost$ webster_min $/card recoveries notdone_batches verify_gate_reports webster_review_rounds review_findings gating_findings",nm," ")
  split("4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19",cs," ")
  printf "| metric (median) | cautious | cautious >=10 cards | wide |\n|---|---|---|---|\n"
  printf "| runs | %d | %d | %d |\n",G["cautious"],G["cautious>=10cards"],G["wide"]
  for(i=1;i<=16;i++){ c=cs[i]; r=nm[i]; row="| " r " (n"
    out=""; split("cautious cautious>=10cards wide",gs," ")
    for(j=1;j<=3;j++){ g=gs[j]; n=N[g,c]+0; for(k=1;k<=n;k++)tmp[k]=V[g,c,k]; out=out " | " med(tmp,n) " (n=" n ")"; delete tmp }
    print "| " r out " |" }
}' "$OUT/runs.tsv" > "$OUT/summary.md"

# fork-level: peak/estimate and forks over the cautious budget, per group
awk -F'\t' 'NR>1 && $7!="NA"{g=($2~/wide/)?"wide":"cautious"; n[g]++; r[g]+=$7; if($6>200000)o2[g]++; if($6>300000)o3[g]++; if($7>1)u[g]++}
 END{print "\n| fork-level | cautious | wide |\n|---|---|---|"; printf "| forks with estimate+peak | %d | %d |\n",n["cautious"],n["wide"]; printf "| mean peak/estimate | %.2f | %.2f |\n",r["cautious"]/n["cautious"],r["wide"]/n["wide"]; printf "| forks peaking over 200k | %d | %d |\n",o2["cautious"],o2["wide"]; printf "| forks peaking over 300k | %d | %d |\n",o3["cautious"],o3["wide"]; printf "| forks peaking above their estimate | %d | %d |\n",u["cautious"],u["wide"]}' "$OUT/forks.tsv" >> "$OUT/summary.md"
cat "$OUT/summary.md"
