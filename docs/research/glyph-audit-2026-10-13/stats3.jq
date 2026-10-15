def med: sort|if length==0 then null else (if length%2==1 then .[length/2|floor] else ((.[length/2-1]+.[length/2])/2) end) end;
def arm: if .raf then "raf" elif .pcard then "mid" else "base_on" end;
[.[]|select(.cards>0)] | group_by(arm)[] | (.[0]|arm) as $k
| [$k, length, "nsf_med=\(map(.nsf)|med)", "pre_hit_share_med=\(map(select(.nsf>0)|.pre_hit/.nsf*100|round)|med)%", "pooled_hit=\((map(.pre_hit)|add)/(map(.nsf)|add)*100|round)%", "edits=\(map(.edits)|add) edits_out=\(map(.edits_out)|add) share=\((map(.edits_out)|add)/(map(.edits)|add)*100|round)%", "forks_with_edit_outside=\(map(select(.edits_out>0))|length)", "writes=\(map(.writes)|add) writes_out=\(map(.writes_out)|add)"]|@tsv
