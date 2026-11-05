def med: sort | if length==0 then null else (if length%2==1 then .[length/2|floor] else ((.[length/2-1]+.[length/2])/2) end) end;
def mean: if length==0 then null else (add/length) end;
def r: if .==null then "-" else (.*10|round/10|tostring) end;
[ .[] | select(.fork and .cards>0 and .start>=$from) ] as $s
| ($s|map(select(.pack))) as $on | ($s|map(select(.pack|not))) as $off
| def row($k): [$k, ($on|map(.[$k]//empty)|med|r), ($off|map(.[$k]//empty)|med|r), ($on|map(.[$k]//empty)|mean|r), ($off|map(.[$k]//empty)|mean|r)] | @tsv;
  def prow($k): [$k+"/card", ($on|map(.[$k]/.cards)|med|r), ($off|map(.[$k]/.cards)|med|r), ($on|map(.[$k]/.cards)|mean|r), ($off|map(.[$k]/.cards)|mean|r), "pooled", (($on|map(.[$k])|add)/($on|map(.cards)|add)|r), (($off|map(.[$k])|add)/($off|map(.cards)|add)|r)] | @tsv;
"n_on=\($on|length) runs_on=\($on|map(.proj)|unique|length) cards_on=\($on|map(.cards)|add) n_off=\($off|length) runs_off=\($off|map(.proj)|unique|length) cards_off=\($off|map(.cards)|add)",
"metric\tmedON\tmedOFF\tmeanON\tmeanOFF",
(["cards","pre_edit","tools","turns","grep","grep_pre","gmem","gmem_pre","quarry","rfull","rfull_pre","rpart","rpart_pre","ex_segs","ex_tools","ex_grep","ex_reads","out","fresh_in","cache_read"][] | row(.)),
(["ex_tools","ex_grep","ex_reads","tools","turns","grep","gmem","rfull","rpart","out","fresh_in"][] | prow(.))
