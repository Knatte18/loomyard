# Use: jq -s -r -f quartiles.jq raw.jsonl   -> min, q1, median, q3, max of per-fork metrics per arm (nearest-rank quartiles)
def q($p): sort | .[(((length-1)*$p)|round)];
def r: (.*10|round/10);
[ .[] | select(.cards>0)
  | {layout, raf,
     "ex_net/card": (.ex_net/.cards), "rfull_net/card": (.rfull_net/.cards), "turns/card": (.turns/.cards),
     "out_k/card": (.out/.cards/1000), "cache_read_M/card": (.cache_read/.cards/1e6),
     "pre_edit_net": .pre_edit_net, "first_par": .first_par,
     "strict span share pre-edit %": (if .nspans>0 then .spans_strict_pre/.nspans*100 else null end),
     "edits/card": (.edits/.cards)} ] as $s
| {pf: ($s|map(select(.layout=="file"))), raf: ($s|map(select(.layout=="inline" and .raf)))} as $a
| (["metric","arm","n","min","q1","median","q3","max"] | @tsv),
  ( ["ex_net/card","rfull_net/card","turns/card","out_k/card","cache_read_M/card","pre_edit_net","first_par","strict span share pre-edit %","edits/card"][] as $m
    | ("pf","raf") as $k
    | ($a[$k] | map(.[$m]) | map(select(.!=null))) as $v
    | [$m, $k, ($v|length), ($v|min|r), ($v|q(0.25)|r), ($v|q(0.5)|r), ($v|q(0.75)|r), ($v|max|r)] | @tsv )
