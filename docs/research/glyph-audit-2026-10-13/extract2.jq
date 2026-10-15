def rtext: if type=="array" then map(.text // "")|join("") elif type=="string" then . else "" end;
def segs: (.input.command // "") | [splits("&&|\\|\\||;|\\n")] | map(sub("^\\s+";"")) | map(select(length>0));
def stage0: split("|")[0] | sub("^\\s+";"");
def piped: split("|") | length > 1;
def cls:
  if .name=="Grep" then (if ((.input.pattern//"")|test("func |type |[A-Za-z]*[a-z][A-Z][A-Za-z0-9]*|\\b[A-Z][a-z]+[A-Za-z0-9]*")) then ["grep","gmem"] else ["grep"] end)
  elif .name=="Glob" then ["glob"]
  elif .name=="Read" then (if .input.offset==null and .input.limit==null then ["rfull"] else ["rpart"] end)
  elif .name=="Bash" then [ segs[] |
      if test("^lyx (quarry|glyph)") then "quarry"
      elif (stage0|test("^(git )?(grep|rg)( |$)")) then ("grep", (select(stage0|test("func |type |[A-Za-z]*[a-z][A-Z][A-Za-z0-9]*|\\b[A-Z][a-z]+[A-Za-z0-9]*"))|"gmem"))
      elif (stage0|test("^cat [^<]")) and (test("\\.go|\\.md|\\.ya?ml|/")) then (if piped and test("\\| *(head|tail|sed|awk|grep)") then "rpart" else "rfull" end)
      elif (stage0|test("^(awk|sed -n|head|tail|nl) ")) and (piped|not) then "rpart"
      elif (stage0|test("^(ls|find|tree)( |$)")) then "list"
      else empty end ]
  else [] end;
. as $all
| ([ $all[] | select(.type=="user") | .message.content | if type=="array" then .[] | select(.type=="tool_result") | (.content|rtext) else empty end ] | map(select(test("# Webster implementer job"))) | .[0]) as $p
| if $p == null then empty else
([ $all[] | select(.type=="user" and (.message.content|type)=="string") | .message.content ] | map(select(test("^Load these skills")|not)) | .[0] // "") as $first
| ($p | split("## Your cards")[1] // "" | split("For EACH card")[0]) as $cs
| ([ $cs | split("\n")[] | select(test("- `_lyx/plan/\\d+-")) ] | length) as $cards
| ($p | test("## Kick-start pack")) as $pack
| ($p | split("## Kick-start pack")[1] // "" | split("\n")) as $pl
| ([ $pl | reduce .[] as $l ({u:false,res:0,un:0,files:[]}; if ($l|test("^\\s*\\d+\\t### ")) then .u=false elif ($l|test("^\\s*\\d+\\t#### Unresolved")) then .u=true elif ($l|test("^\\s*\\d+\\t- `")) then (if .u then .un+=1 else .res+=1 end) elif ($l|test("^\\s*\\d+\\t  - `[^`:]+:\\d+-\\d+`")) then .files += [($l|capture("^\\s*\\d+\\t  - `(?<f>[^`:]+):").f)] else . end) ] | .[0]) as $pk
| ($pk.res) as $pres
| ($pk.un) as $punres
| ([ $all[] | select(.type=="assistant") | .message.content[]? | select(.type=="tool_use") ]) as $tu0
| ($tu0 | map(select(.name!="Skill" and .name!="Agent"))) as $tu1
| (if ($tu1|length)>0 and $tu1[0].name=="Read" and (($tu1[0].input.file_path//"")|test("prompt\\.md$|/webster/prompts/")) then $tu1[1:] else $tu1 end) as $tu
| ([ $tu | to_entries[] | select(.value.name|test("^(Edit|Write|MultiEdit|NotebookEdit)$")) | .key ] | .[0]) as $fe
| ($tu | if $fe==null then . else .[0:$fe] end) as $pre
| (reduce $tu[] as $t ({ex:true, n:0, g:0, r:0, segs:0};
    if ($t.name|test("^(Edit|Write|MultiEdit|NotebookEdit)$")) then (if .ex then .segs+=1 else . end) | .ex=false
    elif ($t.name=="Bash" and (($t.input.command//"")|test("git commit"))) then .ex=true
    elif .ex then .n+=1 | .g += ([$t|cls[]|select(.=="grep")]|length) | .r += ([$t|cls[]|select(.=="rfull" or .=="rpart")]|length)
    else . end)) as $ex
| ([ $tu[] | cls[] ]) as $c
| ([ $pre[] | cls[] ]) as $cp
| ([ $tu[] | select(.name|test("^(Edit|MultiEdit)$")) | (.input.file_path//"") ]) as $ed | ([ $tu[] | select(.name=="Write") | (.input.file_path//"") ]) as $wr | ($pk.files|unique) as $sf | def outside($l): $l | map(select(. as $e | ($sf|map(. as $s | ($e == $s or ($e|endswith("/"+$s))))|any)|not)) | length;
  def cnt($a; $k): $a | map(select(.==$k)) | length;
  ([ $all[] | select(.type=="assistant" and .message.id!=null) ] | group_by(.message.id) | map(.[-1])) as $msgs
| { proj: (input_filename|split("/")|map(select(startswith("-home")))[0]|sub("-home-knatte-Code-loomyard-LYXHUB-";"")), fork: (input_filename|test("/subagents/")), sid: (input_filename|sub("^\\./";"")),
    start: ($all|map(.timestamp//empty)|min),
    model: ($msgs|map(.message.model)|unique|map(select(.!="<synthetic>"))|join(",")), pack: $pack, nsf: ($sf|length), pre_hit: ([ $sf[] as $s | select([ $pre[] | ((.input.file_path//"") + " " + (.input.command//"")) | contains($s) ] | any) ] | length), raf: ($p|test("Your first turn, before any edit, reads in parallel every card file")), pcard: ($p|test("These refs of this card did not resolve|no member glyphs on this card|Each card.s subsection sits")), edits: ($ed|length), edits_out: outside($ed), writes: ($wr|length), writes_out: outside($wr), pack_res: $pres, pack_unres: $punres, cards: $cards, shuttle: ($first|test("prompt\\.md")),
    tools: ($tu|length), pre_edit: $fe,
    grep: cnt($c;"grep"), grep_pre: cnt($cp;"grep"), gmem: cnt($c;"gmem"), gmem_pre: cnt($cp;"gmem"), quarry: cnt($c;"quarry"), quarry_pre: cnt($cp;"quarry"),
    rfull: cnt($c;"rfull"), rpart: cnt($c;"rpart"), rfull_pre: cnt($cp;"rfull"), rpart_pre: cnt($cp;"rpart"),
    turns: ($msgs|length), ex_tools: $ex.n, ex_grep: $ex.g, ex_reads: $ex.r, ex_segs: $ex.segs,
    out: ($msgs|map(.message.usage.output_tokens//0)|add),
    fresh_in: ($msgs|map((.message.usage.input_tokens//0)+(.message.usage.cache_creation_input_tokens//0))|add),
    cache_read: ($msgs|map(.message.usage.cache_read_input_tokens//0)|add) }
end
