def med: sort | if length==0 then null else (if length%2==1 then .[length/2|floor] else ((.[length/2-1]+.[length/2])/2) end) end;
def r: if .==null then "-" else (.*10|round/10|tostring) end;
[ .[] | select(.fork and .cards>0) | .reads=(.rfull+.rpart) ] as $s
| {base_off: ($s|map(select(.pack|not))), base_on: ($s|map(select(.pack and (.pcard|not)))), mid: ($s|map(select(.pcard and (.raf|not)))), raf: ($s|map(select(.raf)))} as $a
| ([
  ["arm","n","runs","cards"],
  ($a|to_entries[]|[.key,(.value|length),(.value|map(.proj)|unique|length),(.value|map(.cards)|add)]),
  [],
  (["metric"]+($a|keys)),
  (["pre_edit raw med"]+[$a[]|map(.pre_edit//empty)|med|r]),
  (["pre_edit/card med"]+[$a[]|map(select(.pre_edit!=null)|.pre_edit/.cards)|med|r]),
  (["ex_tools","ex_grep","ex_reads","gmem","grep","quarry","rfull","rpart","reads","tools","turns","out","fresh_in","cache_read"][] as $k | ["\($k)/card med"]+[$a[]|map(.[$k]/.cards)|med|r]),
  (["ex_tools","rfull","rpart","grep","reads","turns","out"][] as $k | ["\($k) pooled/card"]+[$a[]|((map(.[$k])|add)/(map(.cards)|add))|r]),
  (["rfull_pre","rpart_pre","grep_pre","quarry_pre"][] as $k | ["\($k) raw med"]+[$a[]|map(.[$k])|med|r]),
  (["forks w/ quarry>0"]+[$a[]|map(select(.quarry>0))|length]),
  (["pack_res med"]+[$a[]|map(select(.pack)|.pack_res)|med|r]),
  (["pack_unres med"]+[$a[]|map(select(.pack)|.pack_unres)|med|r]),
  (["pack_res/card med"]+[$a[]|map(select(.pack)|.pack_res/.cards)|med|r]),
  (["pack_unres/card med"]+[$a[]|map(select(.pack)|.pack_unres/.cards)|med|r]),
  (["unres share pooled"]+[$a[]|(map(select(.pack)|.pack_unres)|add) as $u|(map(select(.pack)|.pack_res)|add) as $v|if $v==null then "-" else ($u/($u+$v)*100|round|tostring)+"%" end]),
  (["pack_res==0 forks"]+[$a[]|map(select(.pack and .pack_res==0))|length]),
  (["edits/fork med"]+[$a[]|map(.edits)|med|r])
 ] | .[] | @tsv)
