# analyze.jq: reads runs.tsv as one raw string (jq -R -s) and prints the report's tables as markdown.
# Times are shown in minutes, sizes in changed lines (added plus deleted); n stands beside every figure.
# A period is set by a run's start against the commits that cut it, which analyze.sh passes in as $cut_*.

def num: if . == "-" or . == "" then null else tonumber end;
def present: map(select(. != null));
def med: sort | if length == 0 then null elif length % 2 == 1 then .[length / 2 | floor] else (.[length / 2 - 1] + .[length / 2]) / 2 end;
def mean: if length == 0 then null else add / length end;
def r1: if . == null then "-" else (. * 10 | round / 10 | tostring) end;
def r2: if . == null then "-" else (. * 100 | round / 100 | tostring) end;
def mins: if . == null then null else . / 60 end;
def sum: if length == 0 then 0 else add end;

# cell: n, mean, median and range of a list of seconds, in minutes.
def cell: present as $v | if ($v | length) == 0 then ["0", "-", "-", "-"]
  else [($v | length | tostring), ($v | mean | mins | r1), ($v | med | mins | r1), "\($v | min | mins | r1)–\($v | max | mins | r1)"] end;
# cell_n: the same for a plain count, such as lines, shown unconverted.
def cell_n: present as $v | if ($v | length) == 0 then ["-", "-", "-"]
  else [($v | mean | r1), ($v | med | r1), "\($v | min | r1)–\($v | max | r1)"] end;

# fit: least squares of active minutes on changed lines per 100, over the runs holding both; [fixed minutes, minutes per 100 lines, correlation, n].
def fit:
  map(select(.lines != null and .active != null) | [.lines / 100, .active / 60]) as $p
  | ($p | length) as $n
  | if $n < 3 then ["-", "-", "-", ($n | tostring)] else
    ($p | map(.[0]) | add) as $sx | ($p | map(.[1]) | add) as $sy
    | ($p | map(.[0] * .[0]) | add) as $sxx | ($p | map(.[0] * .[1]) | add) as $sxy | ($p | map(.[1] * .[1]) | add) as $syy
    | ($n * $sxx - $sx * $sx) as $dx | ($n * $syy - $sy * $sy) as $dy
    | if $dx == 0 or $dy == 0 then ["-", "-", "-", ($n | tostring)] else
      (($n * $sxy - $sx * $sy) / $dx) as $b | (($sy - $b * $sx) / $n) as $a
      | [($a | r1), ($b | r1), (($n * $sxy - $sx * $sy) / (($dx * $dy) | sqrt) | r2), ($n | tostring)] end end;

# per100: pooled minutes per 100 changed lines (sum of active over sum of lines) and the median of the runs' own ratios.
def per100:
  map(select(.lines != null and .lines > 0 and .active != null)) as $v
  | if ($v | length) == 0 then ["-", "-"]
    else [(($v | map(.active) | add) / ($v | map(.lines) | add) * 100 / 60 | r1), ($v | map(.active / .lines * 100 / 60) | med | r1)] end;

def table($head; $rows):
  ["| " + ($head | join(" | ")) + " |", "|" + ($head | map(" --- |") | join(""))] + ($rows | map("| " + (. | join(" | ")) + " |")) | join("\n");

def period:
  if .start < $cut_before then "Before"
  elif .start > $cut_after then "After"
  else "Between" end;
def stage:
  if .period != "After" then .period
  elif .start <= $hook then "After 1: before the hook"
  elif .start <= $widened then "After 2: hook"
  elif .start <= $explore then "After 3: hook widened"
  else "After 4: Discussion exploration" end;

(split("\n") | map(select(length > 0) | split("\t"))) as $t
| $t[0] as $h
| ($t[1:] | map([$h, .] | transpose | map({(.[0]): .[1]}) | add)) as $raw
| ($raw | map(
    . as $r
    | ["span", "sleep", "active", "esc", "halt", "parent_review", "pr_gate", "setup", "discussion", "plan", "webster", "write", "review", "merge", "publish_finalize", "land", "paused", "other", "lines", "gate_wall", "gate_wait", "writer_gate_wall", "gate_runs"]
    | reduce .[] as $k ($r; .[$k] |= num)
    | .waits = ((.esc // 0) + (.halt // 0) + (.parent_review // 0) + (.pr_gate // 0))
    | .period = period
    | .stage = stage
    | .bench = ((.profile == "docs") or (.slug | test("bench|trial|audit|measure|survey|spike|remeasure")))
    | .slept_all = (.span != null and .span > 0 and .sleep != null and .sleep >= .span)
  )) as $runs
| ($runs | map(select(.recipe == "darn"))) as $darn
| ($darn | map(select(.bench | not))) as $code_all
| ($code_all | map(select(.slept_all | not))) as $code
| ($runs | map(select(.recipe == "tapestry"))) as $tap
| ["Before", "Between", "After", "After 1: before the hook", "After 2: hook", "After 3: hook widened", "After 4: Discussion exploration"] as $stages
| def of($set; $s): $set | map(select(.period == $s or .stage == $s));
  def timerow($label; $set):
    [$label] + ($set | map(.active) | cell) + [($set | map(.span) | present | mean | mins | r1), ($set | map(.sleep) | present | mean | mins | r1), ($set | map(.waits) | mean | mins | r1)]
      + ($set | map(.lines) | cell_n) + ($set | per100) + ($set | fit);
  def timehead: ["Period", "n", "Active mean (min)", "median", "range", "Span mean", "Sleep mean", "Waits mean", "Lines mean", "median", "range", "min per 100 lines (pooled)", "median of ratios", "Fit fixed (min)", "per 100 lines (min)", "r", "fit n"];

  "## Runs by period\n",
  table(["Recipe", "Before", "Between", "After", "After 1", "After 2", "After 3", "After 4"];
    [["darn (all)"] + ($stages | map(. as $s | of($darn; $s) | length | tostring)),
     ["darn (bench or docs by name)"] + ($stages | map(. as $s | of($darn | map(select(.bench)); $s) | length | tostring)),
     ["darn (code, machine sleep covers the whole span: no active time)"] + ($stages | map(. as $s | of($code_all | map(select(.slept_all)); $s) | length | tostring)),
     ["darn (code, active time measured)"] + ($stages | map(. as $s | of($code; $s) | length | tostring)),
     ["darn (code, no machine sleep at all)"] + ($stages | map(. as $s | of($code | map(select(.sleep == 0)); $s) | length | tostring)),
     ["tapestry"] + ($stages | map(. as $s | of($tap; $s) | length | tostring))]),
  "",
  "## Darn code runs: active time and size\n",
  table(timehead; $stages | map(. as $s | timerow($s; of($code; $s)))),
  "",
  "## Darn code runs with no machine sleep at all: active time and size\n",
  table(timehead; $stages | map(. as $s | timerow($s; of($code | map(select(.sleep == 0)); $s)))),
  "",
  "## Darn code runs by size band: median active minutes (n)\n",
  table(["Lines changed", "Before", "After", "After / Before"];
    [[0, 100, "< 100"], [100, 300, "100–299"], [300, 1000, "300–999"], [1000, 1e9, "≥ 1000"]]
    | map(. as [$lo, $hi, $label]
      | ($code | map(select(.lines != null and .lines >= $lo and .lines < $hi))) as $band
      | ($band | map(select(.period == "Before") | .active) | present) as $b
      | ($band | map(select(.period == "After") | .active) | present) as $a
      | [$label, "\($b | med | mins | r1) (\($b | length))", "\($a | med | mins | r1) (\($a | length))",
         (if ($b | length) > 0 and ($a | length) > 0 and ($b | med) > 0 then (($a | med) / ($b | med) | r2) else "-" end)])),
  "",
  "## Darn code runs by kind of landing: median active minutes and lines (n)\n",
  table(["Kind", "Before active", "Before lines", "After active", "After lines"];
    ["fix", "feature", "other", "-"]
    | map(. as $k
      | ($code | map(select(.kind == $k))) as $s
      | ($s | map(select(.period == "Before"))) as $b | ($s | map(select(.period == "After"))) as $a
      | [(if $k == "-" then "unmapped" else $k end),
         "\($b | map(.active) | present | med | mins | r1) (\($b | length))", ($b | map(.lines) | present | med | r1),
         "\($a | map(.active) | present | med | mins | r1) (\($a | length))", ($a | map(.lines) | present | med | r1)])),
  "",
  "## Darn code runs: phases (minutes; n counts the runs holding the phase)\n",
  table(["Phase", "Before n", "mean", "median", "range", "After n", "mean", "median", "range"];
    ["setup", "write", "review", "merge", "publish_finalize", "land"]
    | map(. as $p
      | ([$code[] | select(.period == "Before") | .[$p]] | cell) as $b | ([$code[] | select(.period == "After") | .[$p]] | cell) as $a
      | [$p] + $b + $a)),
  "",
  "## Darn code runs: what active time leaves out (mean minutes over all runs of the period)\n",
  table(["Period", "n", "Span", "Sleep", "Escalation", "Halt", "Parent review", "PR gate", "Paused phase", "Active"];
    ["Before", "After"] | map(. as $s | $code | map(select(.period == $s)) as $set
      | [$s, ($set | length | tostring)] + (["span", "sleep", "esc", "halt", "parent_review", "pr_gate", "paused", "active"] | map(. as $k | $set | map(.[$k] // 0) | mean | mins | r1)))),
  "",
  "## Darn code runs: recipe rows (share of runs holding the row, and mean visits per run)\n",
  ($prod | split("\n") | map(select(length > 0) | split("\t") | {key: .[1], value: (.[4] | split(","))}) | from_entries) as $rows
  | ($code | map(.rows = $rows[.tag])) as $coded
  | ($coded | map(.rows[]) | unique) as $names
  | table(["Row", "Before share", "visits", "After share", "visits"];
      $names | map(. as $name
        | ($coded | map(select(.period == "Before"))) as $b | ($coded | map(select(.period == "After"))) as $a
        | [$name,
           (($b | map(select(.rows | index($name))) | length) / ($b | length) * 100 | round | tostring + "%"), (($b | map(.rows | map(select(. == $name)) | length) | mean) | r2),
           (($a | map(select(.rows | index($name))) | length) / ($a | length) * 100 | round | tostring + "%"), (($a | map(.rows | map(select(. == $name)) | length) | mean) | r2)])
        | select(.[1] != .[3] or .[2] != .[4])),
  "",
  "## Concurrency: other landed runs in flight at a run's start (mean, all recipes)\n",
  ($runs | map(. + {t0: (.start | fromdateiso8601)}) | map(. + {t1: (.t0 + (.span // 0))})) as $timed
  | table(["Period", "Runs", "Mean runs in flight at start"];
      ["Before", "After"] | map(. as $s
        | ($timed | map(select(.period == $s))) as $set
        | [$s, ($set | length | tostring), ($set | map(. as $r | $timed | map(select(.tag != $r.tag and .t0 <= $r.t0 and .t1 > $r.t0)) | length) | mean | r1)])),
  "",
  "## Darn code runs: gate time (minutes; runs with a gate report row)\n",
  table(["Period", "n", "Gate wall mean", "median", "Slot wait mean", "Writer gate wall mean", "Active mean", "Active less gate wall, mean", "median", "Gate runs mean"];
    ["Before", "After"] | map(. as $s | $code | map(select(.period == $s and .gate_wall != null)) as $set
      | [$s, ($set | length | tostring), ($set | map(.gate_wall) | mean | mins | r1), ($set | map(.gate_wall) | med | mins | r1), ($set | map(.gate_wait) | mean | mins | r1),
         ($set | map(.writer_gate_wall) | mean | mins | r1), ($set | map(.active) | mean | mins | r1),
         ($set | map(.active - .gate_wall) | mean | mins | r1), ($set | map(.active - .gate_wall) | med | mins | r1), ($set | map(.gate_runs) | mean | r1)])),
  "",
  "## Darn runs apart: bench, measurement and docs runs\n",
  table(["Slug", "Period", "Profile", "Active (min)", "Lines"];
    $darn | map(select(.bench)) | sort_by(.start)
    | map([.slug, .period, .profile, (.active | mins | r1), (.lines | if . == null then "-" else tostring end)])),
  "",
  "## Darn runs whose profile is recorded\n",
  table(["Profile", "Period", "n", "Active mean (min)", "median", "Lines median"];
    $darn | map(select(.profile != "-")) | group_by([.profile, .period]) | map([.[0].profile, .[0].period, (length | tostring), (map(.active) | mean | mins | r1), (map(.active) | med | mins | r1), (map(.lines) | present | med | r1)])),
  "",
  "## Tapestry runs\n",
  table(["Run", "Start", "Period", "Class", "Lines", "Active", "Setup", "Discussion", "Plan", "Webster", "Merge", "Publish and Finalize", "Land", "Waits", "Sleep", "Gate wall"];
    $tap | sort_by(.start)
    | map((if .slug == "cold-gate-fixer" then "excluded: no Discussion or Plan"
           elif .discussion == null and .stored_record == "true" then "Sketch-fed"
           else "whole run" end) as $class
      | [.slug, (.start | .[0:16]), .stage, $class, (.lines | if . == null then "-" else tostring end), (.active | mins | r1),
         (.setup | mins | r1), (.discussion | mins | r1), (.plan | mins | r1), (.webster | mins | r1), (.merge | mins | r1), (.publish_finalize | mins | r1), (.land | mins | r1),
         (.waits | mins | r1), (.sleep | mins | r1), (.gate_wall | mins | r1)])),
  "",
  "## Tapestry: whole runs with a Discussion, and Plan through Land for every run but cold-gate-fixer (minutes)\n",
  ($tap | map(select(.slug != "cold-gate-fixer"))) as $tp
  | ($tp | map(select(.discussion != null))) as $tw
  | table(["Measure", "Before n", "mean", "median", "range", "After n", "mean", "median", "range"];
      [["Whole run, active", ($tw | map(select(.period == "Before") | .active) | cell), ($tw | map(select(.period == "After") | .active) | cell)],
       ["Discussion phase", ($tw | map(select(.period == "Before") | .discussion) | cell), ($tw | map(select(.period == "After") | .discussion) | cell)],
       ["Plan through Land, phase sum (waits inside)", ($tp | map(select(.period == "Before") | .plan + .webster + (.merge // 0) + .publish_finalize + .land) | cell), ($tp | map(select(.period == "After") | .plan + .webster + (.merge // 0) + .publish_finalize + .land) | cell)]]
      | map([.[0]] + .[1] + .[2])),
  "",
  "## Tapestry phases (minutes; n counts the runs holding the phase, cold-gate-fixer and Sketch-fed runs left out of Discussion and Plan)\n",
  table(["Phase", "Before n", "mean", "median", "range", "After n", "mean", "median", "range"];
    ["setup", "discussion", "plan", "webster", "merge", "publish_finalize", "land"]
    | map(. as $p
      | ($tap | map(select(.slug != "cold-gate-fixer")) | map(select(.period == "Before") | .[$p]) | cell) as $b
      | ($tap | map(select(.slug != "cold-gate-fixer")) | map(select(.period == "After") | .[$p]) | cell) as $a
      | [$p] + $b + $a)),
  "",
  "## Tapestry size and fit (whole runs with a Discussion; lines from the landing commit)\n",
  table(["Period", "n", "Lines mean", "median", "min per 100 lines (pooled)", "median of ratios", "Fit fixed (min)", "per 100 lines (min)", "r", "fit n"];
    ["Before", "After"] | map(. as $s | $tw | map(select(.period == $s)) as $set | [$s, ($set | length | tostring)] + ($set | map(.lines) | cell_n | .[0:2]) + ($set | per100) + ($set | fit)))
