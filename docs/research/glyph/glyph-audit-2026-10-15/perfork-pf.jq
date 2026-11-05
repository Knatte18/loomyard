# One row per pf fork: the per-fork figures the report quotes.
(["run","start","cards","pack_turn","first_work_turn","first_par","nspans","spans_strict_pre","spans_file_pre","spans_strict_first","pre_edit","pre_edit_net","ex_net","rfull_net","rpart","grep","quarry","turns","out_k","cache_read_M","edits","edits_out"] | @tsv),
(.[] | select(.layout=="file" and .cards>0)
 | [.proj,.start[5:16],.cards,.pack_turn,.first_work_turn,.first_par,.nspans,.spans_strict_pre,.spans_file_pre,.spans_strict_first,.pre_edit,.pre_edit_net,.ex_net,.rfull_net,.rpart,.grep,.quarry,.turns,(.out/1000|round),(.cache_read/1e6*10|round/10),.edits,.edits_out] | @tsv)
