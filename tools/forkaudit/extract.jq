# extract.jq turns one subagent transcript, read line by line with `jq -n -R`, into the per-fork record forkaudit.sh collects.
# It emits nothing unless the transcript is a Webster implementer fork:
# its first tool call (Skill and Agent calls excluded) must read a Webster prompt file whose result holds the job heading.
# Arguments: $mode ("extract", or "arms" to print the marker table's arm names and read no input),
# $run (the run name of the fork's project directory), $rel (the transcript's path relative to the projects directory)
# and $plan (the plan directory the prompt's card lines name).

# The arm marker table, checked in order, one row per arm; a fork matching no row is "unknown".
# A row matches when every feature in its `when` equals the fork's feature.
# A task that adds a Webster pack layout adds its marker row here in the same task.
def markers: [
  {arm: "file", when: {starthere: true}},
  {arm: "raf", when: {inlinepack: true, rafphrase: true}},
  {arm: "inline", when: {inlinepack: true}},
  {arm: "none", when: {inlinepack: false, packopened: false}}
];

def armof($features):
  (markers | map(select(.when | to_entries | all(. as $e | $features[$e.key] == $e.value))) | .[0].arm) // "unknown";

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
# hit($t; $s): 0 = no mention, 1 = file-level read of the span's file, 2 = the read reaches the span (Read overlap, whole cat, sed -n or head range overlap)
def hit($t; $s):
  if $t.name=="Read" then
    ($t.input.file_path//"") as $p
    | if ($p==$s.f or ($p|endswith("/"+$s.f))) then
        (if $t.input.offset==null and $t.input.limit==null then 2
         else (($t.input.offset//1) as $o | ($o + ($t.input.limit//2000) - 1) as $e | if $o<=$s.b and $e>=$s.a then 2 else 1 end) end)
      else 0 end
  elif $t.name=="Bash" then
    ($t.input.command//"") as $c
    | if (($c|contains($s.f)) and ($t|cls|any(.=="rfull" or .=="rpart"))) then
        (if ($c|test("(^|[;&|\\n] *)cat ") and ($c|test("\\| *(head|tail|sed|awk)")|not)) then 2
         elif ([ $c | scan("sed -n '?([0-9]+),([0-9]+)p") | select((.[0]|tonumber)<=$s.b and (.[1]|tonumber)>=$s.a) ] | length) > 0 then 2
         elif ([ $c | scan("head -(?:n *)?([0-9]+)") | select((.[0]|tonumber)>=$s.a) ] | length) > 0 then 2
         else 1 end)
      else 0 end
  else 0 end;

if $mode == "arms" then [markers[].arm] else
([inputs | select(test("\\S"))]) as $lines
| ($lines | map(fromjson? | select(type == "object"))) as $all
| (($lines | length) - ($all | length)) as $dropped
| ([ $all[] | select(.type=="assistant" and .message.id!=null) ]) as $asst
| (reduce $asst[] as $m ({order:[], idx:{}}; ($m.message.id) as $id | if .idx[$id]==null then (.order += [$id]) | .idx[$id]=(.order|length) else . end) | .idx) as $idx
| (if ($asst|length)>0 and ([$asst[0].message.content[]?|select(.type=="tool_use" and .name=="Agent")]|length)>0 then 1 else 0 end) as $off
| ([ $asst[] | .message.id as $id | .message.content[]? | select(.type=="tool_use") | . + {turn: ($idx[$id] - $off)} ]) as $tu0
| ($tu0 | map(select(.name!="Skill" and .name!="Agent"))) as $tu1
| ([ $all[] | select(.type=="user") | .message.content | if type=="array" then .[] | select(.type=="tool_result") | {id: .tool_use_id, t: (.content|rtext)} else empty end ]) as $results
| (reduce $results[] as $r ({}; .[$r.id] = $r.t)) as $rbyid
| (($tu1[0]) as $f0 | if $f0!=null and $f0.name=="Read" and (($f0.input.file_path//"")|test("/webster/prompts/|prompt\\.md$")) and (($f0.input.file_path//"")|test("-pack\\.md$")|not) then $f0 else null end) as $promptread
| (if $promptread==null then null else $rbyid[$promptread.id] end) as $p
| if $p == null or ($p|test("# Webster implementer job")|not) then empty else
(if $promptread==null then $tu1 else $tu1[1:] end) as $tu
| ($tu | map(select(.name=="Read" and ((.input.file_path//"")|test("-pack\\.md$"))))) as $packreads
| ($packreads[0]) as $pr
| ($pr != null) as $packopened
| (if $packopened then $rbyid[$pr.id] else null end) as $packfile
| ($p | test("\n[0-9 ]*\t?## Start here")) as $starthere
| ($p | test("## Kick-start pack")) as $inlinepack
| ($p | test("Your first turn, before any edit, reads in parallel every card file")) as $rafphrase
| (if $packfile != null then $packfile elif $inlinepack then ($p | split("## Kick-start pack")[1]) else "" end) as $ptext
| ($p | split("## Your cards")[1] // "" | split("For EACH card")[0]) as $cs
| ("- `" + $plan) as $cardline
| ([ $cs | split("\n")[] | select(contains($cardline) and (split($cardline)[1] | test("^\\d+-"))) ] | length) as $cards
| ($ptext | split("\n")) as $pl
| ([ $pl | reduce .[] as $l ({u:false,res:0,un:0,spans:[]};
    if ($l|test("^\\s*\\d+\\t### ")) then .u=false
    elif ($l|test("^\\s*\\d+\\t#### Unresolved")) then .u=true
    elif ($l|test("^\\s*\\d+\\t- `")) then (if .u then .un+=1 else .res+=1 end)
    elif ($l|test("^\\s*\\d+\\t  - `[^`:]+:\\d+(-\\d+)?`")) then
      ($l|capture("^\\s*\\d+\\t  - `(?<f>[^`:]+):(?<a>\\d+)(-(?<b>\\d+))?`")) as $c
      | .spans += [{f:$c.f, a:($c.a|tonumber), b:(($c.b // $c.a)|tonumber)}]
    else . end) ] | .[0]) as $pk
| ($pk.spans | unique) as $spans
| ($spans | map(.f) | unique) as $sf
| ([ $tu | to_entries[] | select(.value.name|test("^(Edit|Write|MultiEdit|NotebookEdit)$")) | .key ] | .[0]) as $fe
| ($tu | if $fe==null then . else .[0:$fe] end) as $pre
| (if $promptread!=null then $promptread.turn else 0 end) as $prompt_turn
| (if $packopened then $pr.turn else $prompt_turn end) as $base_turn
| ([ $tu[] | select(.turn==$base_turn and .id!=($pr.id//"")) ] | length) as $par_with_pack
| (if $packopened and $par_with_pack>0 then $base_turn else ([ $tu[] | select(.turn>$base_turn) | .turn ] | min) end) as $fwt
| ([ $tu[] | select(.turn==$fwt and .id!=($pr.id//"")) ]) as $fcalls
| ([ $spans[] as $s | ([ $pre[] | select(hit(.; $s)>=1) | .turn ] | min) as $t1 | ([ $pre[] | select(hit(.; $s)>=2) | .turn ] | min) as $t2 | {t1:$t1, t2:$t2} ]) as $sh
| (reduce $tu[] as $t ({ex:true, n:0, g:0, r:0, segs:0};
    if ($t.name|test("^(Edit|Write|MultiEdit|NotebookEdit)$")) then (if .ex then .segs+=1 else . end) | .ex=false
    elif ($t.name=="Bash" and (($t.input.command//"")|test("git commit"))) then .ex=true
    elif .ex then .n+=1 | .g += ([$t|cls[]|select(.=="grep")]|length) | .r += ([$t|cls[]|select(.=="rfull" or .=="rpart")]|length)
    else . end)) as $ex
| ([ $tu[] | cls[] ]) as $c
| ([ $pre[] | cls[] ]) as $cp
| ([ $tu[] | select(.name|test("^(Edit|MultiEdit)$")) | (.input.file_path//"") ]) as $ed
| ([ $tu[] | select(.name=="Write") | (.input.file_path//"") ]) as $wr
| def outside($l): $l | map(select(. as $e | ($sf|map(. as $s | ($e == $s or ($e|endswith("/"+$s))))|any)|not)) | length;
  def cnt($a; $k): $a | map(select(.==$k)) | length;
  def lastmsgs: ([ $all[] | select(.type=="assistant" and .message.id!=null) ] | group_by(.message.id) | map(.[-1]));
  (lastmsgs) as $msgs
| ($pr != null and ($pre | map(.id) | index($pr.id)) != null) as $pack_pre
| ($packopened | if . then 1 else 0 end) as $pk1
| { proj: $run,
    sid: ($rel | split("/")[1]),
    agent: ($rel | split("/") | last | sub("\\.jsonl$"; "")),
    rel: $rel,
    start: ($all|map(.timestamp//empty)|min),
    arm: armof({starthere: $starthere, inlinepack: $inlinepack, rafphrase: $rafphrase, packopened: $packopened}),
    cards: $cards,
    pack_opened: $packopened, pack_turn: (if $packopened then $pr.turn else null end), pack_reads: ($packreads|length), pack_before_edit: $pack_pre, par_with_pack: $par_with_pack,
    pack_res: $pk.res, pack_unres: $pk.un, nspans: ($spans|length), nsf: ($sf|length),
    first_work_turn: $fwt, first_par: ($fcalls|length),
    first_classes: ([ $fcalls[] | cls[] ] | group_by(.) | map({(.[0]): length}) | add),
    spans_file_pre: ([ $sh[] | select(.t1!=null) ] | length), spans_strict_pre: ([ $sh[] | select(.t2!=null) ] | length),
    spans_file_first: ([ $sh[] | select(.t1!=null and .t1<=$fwt) ] | length), spans_strict_first: ([ $sh[] | select(.t2!=null and .t2<=$fwt) ] | length),
    spans_strict_max_turn: (([ $sh[] | select(.t2!=null) | .t2 ] | group_by(.) | map(length) | max) // 0),
    sf_file_pre: ([ $sf[] as $s | select([ $pre[] | select(hit(.; {f:$s,a:0,b:999999})>=1) ] | length > 0) ] | length),
    edits: ($ed|length), edits_out: outside($ed), writes: ($wr|length), writes_out: outside($wr),
    tools: ($tu|length), pre_edit: $fe, pre_edit_net: (if $fe==null then null else $fe - (if $pack_pre then 1 else 0 end) end),
    grep: cnt($c;"grep"), gmem: cnt($c;"gmem"), quarry: cnt($c;"quarry"), rfull: cnt($c;"rfull"), rpart: cnt($c;"rpart"),
    rfull_net: (cnt($c;"rfull") - $pk1),
    grep_pre: cnt($cp;"grep"), quarry_pre: cnt($cp;"quarry"), rfull_pre: cnt($cp;"rfull"), rpart_pre: cnt($cp;"rpart"),
    turns: ($msgs|length), turns_pre: (if $fe==null then null else $tu[$fe].turn end),
    ex_tools: $ex.n, ex_net: ($ex.n - (if $packopened and $pack_pre then 1 else 0 end)), ex_grep: $ex.g, ex_reads: $ex.r, ex_segs: $ex.segs,
    out: ($msgs|map(.message.usage.output_tokens//0)|add),
    fresh_in: ($msgs|map((.message.usage.input_tokens//0)+(.message.usage.cache_creation_input_tokens//0))|add),
    cache_read: ($msgs|map(.message.usage.cache_read_input_tokens//0)|add),
    dropped: $dropped }
end
end
