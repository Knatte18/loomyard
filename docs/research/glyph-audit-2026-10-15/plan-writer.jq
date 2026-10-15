# One TSV row for a session transcript: counts of the tools it used. Use: jq -s -r -f plan-writer.jq <session.jsonl>
# columns: session, first_ts, plan_card_writes, q_glyphs, q_resolve, q_expand, q_toc, q_other, grep_tool, bash_grep, read_tool, bash_read, glob_ls, total_tools
(input_filename|split("/")|last|sub("\\.jsonl$";"")) as $s
| [ .[] | select(.type=="assistant") | .message.content[]? | select(.type=="tool_use") ] as $t
| ([ $t[] | select(.name=="Bash") | (.input.command//"") | [splits("&&|\\|\\||;|\\n")] | .[] | sub("^\\s+";"") ]) as $segs
| def q($w): [ $segs[] | select(test("^lyx (quarry|glyph) " + $w)) ] | length;
  [ $s, ([.[]|.timestamp//empty]|min),
    ([ $t[] | select(.name=="Write" and ((.input.file_path//"")|test("(^|/)_lyx/plan/[0-9][0-9]-"))) ]|length),
    q("glyphs"), q("resolve"), q("expand"), q("toc"),
    ([ $segs[] | select(test("^lyx (quarry|glyph)")) ] | length) - (q("glyphs")+q("resolve")+q("expand")+q("toc")),
    ([ $t[] | select(.name=="Grep") ]|length),
    ([ $segs[] | select(test("^(git )?(grep|rg)( |$)")) ]|length),
    ([ $t[] | select(.name=="Read") ]|length),
    ([ $segs[] | select(test("^(cat|sed -n|head|tail|nl|awk) ")) ]|length),
    ([ $t[] | select(.name=="Glob") ]|length) + ([ $segs[] | select(test("^(ls|find|tree)( |$)")) ]|length),
    ($t|length) ] | @tsv
