# Use: jq -R -s -r -f plan-stats.jq plan-refs.tsv     (one-column arm summary of the plan reference census)
def med: map(select(.!=null)) | sort | if length==0 then null else (if length%2==1 then .[length/2|floor] else ((.[length/2-1]+.[length/2])/2) end) end;
def r: if .==null then "-" else (.*100|round/100|tostring) end;
def pct($a;$b): if $b==0 then "-" else (($a/$b*100)|round|tostring)+"%" end;
[ split("\n")[] | select(length>0) | split("\t") | {arm:.[0], proj:.[1], c:.[2], card:.[3], label:.[4], kind:.[5], ref:.[6]} ] as $rows
| ($rows | group_by([.arm,.proj,.c,.card]) | map({arm:.[0].arm, proj:.[0].proj, c:.[0].c, card:.[0].card,
    glyph: (map(select(.kind=="glyph"))|length), plan: (map(select(.kind=="planhandle"))|length),
    bfile: (map(select(.kind=="bare_file"))|length), bdir: (map(select(.kind=="bare_dir"))|length),
    n: length})) as $cards
| ($cards | group_by(.arm) | map({key:.[0].arm, value:.}) | from_entries) as $a
| ([ ["metric","pf","raf"],
    (["plans (runs)"] + [("pf","raf") as $k | $a[$k] | map(.proj+.c)|unique|length]),
    (["cards"] + [("pf","raf") as $k | $a[$k] | length]),
    (["refs total"] + [("pf","raf") as $k | $a[$k] | map(.n)|add]),
    (["glyph refs (pkg#Name, existing)"] + [("pf","raf") as $k | $a[$k] | map(.glyph)|add]),
    (["plan: handles (glyph for a member the card creates)"] + [("pf","raf") as $k | $a[$k] | map(.plan)|add]),
    (["bare file paths"] + [("pf","raf") as $k | $a[$k] | map(.bfile)|add]),
    (["bare directory paths"] + [("pf","raf") as $k | $a[$k] | map(.bdir)|add]),
    (["glyph share of refs (glyph+plan)"] + [("pf","raf") as $k | $a[$k] | pct((map(.glyph+.plan)|add); (map(.n)|add))]),
    (["glyph refs / card (median)"] + [("pf","raf") as $k | $a[$k] | map(.glyph)|med|r]),
    (["plan: handles / card (median)"] + [("pf","raf") as $k | $a[$k] | map(.plan)|med|r]),
    (["bare file / card (median)"] + [("pf","raf") as $k | $a[$k] | map(.bfile)|med|r]),
    (["bare dir / card (median)"] + [("pf","raf") as $k | $a[$k] | map(.bdir)|med|r]),
    (["glyph refs / card (pooled)"] + [("pf","raf") as $k | $a[$k] | ((map(.glyph)|add)/length)|r]),
    (["bare file / card (pooled)"] + [("pf","raf") as $k | $a[$k] | ((map(.bfile)|add)/length)|r]),
    (["cards with no glyph (glyph+plan = 0)"] + [("pf","raf") as $k | $a[$k] | ((map(select(.glyph+.plan==0))|length|tostring)+" of "+(length|tostring))]),
    (["cards with at least one bare file"] + [("pf","raf") as $k | $a[$k] | ((map(select(.bfile>0))|length|tostring)+" of "+(length|tostring))]),
    (["cards with only bare paths (glyph+plan = 0, bare > 0)"] + [("pf","raf") as $k | $a[$k] | ((map(select(.glyph+.plan==0 and .bfile+.bdir>0))|length|tostring)+" of "+(length|tostring))])
  ] | .[] | @tsv),
  "",
  ("per plan (arm, proj, commit, cards, glyph, plan:, bare file, bare dir)"),
  ($cards | group_by([.arm,.proj,.c])[] | [.[0].arm,.[0].proj,.[0].c,length,(map(.glyph)|add),(map(.plan)|add),(map(.bfile)|add),(map(.bdir)|add)] | @tsv)
