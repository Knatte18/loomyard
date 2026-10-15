# report.jq renders the report body from the slurped per-fork records, one markdown line per output string.
# Arguments: $markers, the marker table's rows (arm and agreeing trace values) in table order;
# $batches, $traces and $card_traces, the contents of batches.tsv, traces.tsv and card-traces.tsv, which join each record to its resolved batch, its trace layout and its traced card pack count;
# $pack_check, whether to render the pack-correctness section, with $pack_rows and $missed_rows, the contents of pack-check.tsv and missed-files.tsv;
# $plan_on, whether to render the plan glyph-use section, with $plan_runs, $plan_cards, $plan_refs and $plan_writers, the contents of the plan-*.tsv files.
# Rows and arms sort by arm (marker-table order, then any other arm name alphabetically), run, start time and agent id,
# so two runs over the same input differ only in the generation-time line.
#
# Cross-check: a trace layout agrees with a fork when the fork's arm lists it, and any other pairing is a disagreement,
# listed per fork; the arm stays the prompt's.
# On an unknown fork a trace value of [a-z0-9_-]+ that is no arm name makes the arm trace:<value>;
# any other trace value leaves the fork unknown and is listed once as unrecognized.
# A fork without a resolved batch or a trace line before its start has no cross-check.
#
# Missing-metric rule: a fork lacking a metric is left out of that metric's median and pooled value.
# No edit leaves out every "before the first edit" metric, a pack with no spans every span share,
# arm none every pack metric, and zero cards every per-card metric.
# The card arm has no first-work-turn span share, since its first work turn reads card files only,
# so its spans_*_first are null and the two "spans reached in the first work turn" rows leave it out.
# Its other span shares use the window its records state: a span counts as reached when read after the card-pack call that lists it and before the next edit.
#
# Card pack column: `-` for a fork with no card-pack call and no traced card pack line, `<calls>/<traced>` for a fork with a card-traces.tsv row and either,
# and `<calls>/-` for a fork with calls and no row.
# A fork with a row whose count differs from its call count is listed under Card pack disagreements.
#
# Whenever the card arm is present the report prints a note line under the Pack use, Arm comparison and Per-fork tables,
# and under the Pack correctness heading when that section is on: the first three state how the card arm's metrics differ, the last that only its first card-pack call is graded.
#
# A cell whose sample is smaller than the arm's fork count shows its size as `<value> (n=<k>)`, and a cell with no sample left shows n/a.

def dash: if . == null then "-" else tostring end;
def table($head; $rows):
  ["| " + ($head | join(" | ")) + " |", "| " + ($head | map("---") | join(" | ")) + " |"]
  + [$rows[] | "| " + (map(dash) | join(" | ")) + " |"];

def med: map(select(. != null)) | sort | if length == 0 then null elif length % 2 == 1 then .[length / 2 | floor] else (.[length / 2 - 1] + .[length / 2]) / 2 end;
def q($p): sort | .[((length - 1) * $p) | round];
def n2: if . == null then "n/a" else (. * 100 | round / 100 | tostring) end;
def pct: if . == null then "n/a" else ((. * 100 | round | tostring) + "%") end;

# sized($k; $n; $text): $text, with its sample size when $k of the arm's $n forks have the metric, and n/a when none do.
def sized($k; $n; $text): if $k == 0 then "n/a" elif $k < $n then "\($text) (n=\($k))" else $text end;
# The cell helpers take an arm's records and a per-record filter that yields the metric, or null when the fork lacks it.
def med_cell($recs; f; fmt): [$recs[] | f | select(. != null)] as $v | sized($v | length; $recs | length; ($v | med | fmt));
# f yields [numerator, denominator]; the cell is the sum of numerators over the sum of denominators.
def ratio_cell($recs; f; fmt):
  [$recs[] | f | select(. != null)] as $v
  | ($v | map(.[1]) | add) as $den
  | sized($v | length; $recs | length; (if $den == null or $den == 0 then null else (($v | map(.[0]) | add) / $den) end | fmt));
# f yields a boolean; the cell is the count of true over the count of forks that have the metric.
def count_cell($recs; f):
  [$recs[] | f | select(. != null)] as $v
  | sized($v | length; $recs | length; "\($v | map(select(.)) | length) of \($v | length)");

def packed(f): if .arm == "none" then null else f end;
def edited(f): if .pre_edit == null then null else f end;
def spanned(f): if .nspans == 0 then null else f end;
def per_card($k): if .cards > 0 and .[$k] != null then .[$k] / .cards else null end;
def pooled_card($k): if .cards > 0 and .[$k] != null then [.[$k], .cards] else null end;

# first_turn(f): f, or null for a fork whose first-work-turn span metrics are null.
def first_turn(f): if .spans_strict_first == null then null else f end;

def tsv_rows($raw): $raw | split("\n") | map(select(length > 0) | split("\t"));

($markers | map(.arm)) as $arms
| ($arms + ["unknown"]) as $known
| def armkey: . as $arm | [($arms | index($arm)) // 1000, $arm];
  (tsv_rows($batches) | map({key: .[0], value: .}) | from_entries) as $batch_rows
| (tsv_rows($traces) | map({key: .[0], value: .[2]}) | from_entries) as $trace_layouts
| (tsv_rows($card_traces) | map({key: .[0], value: (.[1] | tonumber)}) | from_entries) as $card_trace_counts
| def crosscheck:
    . as $r
    | ($batch_rows[$r.rel] // null) as $row
    | ($trace_layouts[$r.rel] // null) as $trace
    | ($markers | map(select(.arm == $r.arm)) | .[0]) as $marker
    | $r + {batch: (if $row == null or $row[3] == "-" then null else $row[3] end), trace: $trace}
    | if $trace == null then . + {check: null}
      elif $marker != null then . + {check: (if ($marker.trace | index($trace)) != null then "agrees" else "disagrees" end)}
      elif ($trace | test("^[a-z0-9_-]+$")) and (($known | index($trace)) == null) then . + {arm: ("trace:" + $trace), check: "trace arm"}
      else . + {check: "unrecognized"} end;
  map(crosscheck) as $recs
| ([$recs[].arm] | unique | sort_by(armkey)) as $present
| (($present | index("card")) != null) as $card_present
| ([$recs | sort_by([(.arm | armkey), .proj, .start, .agent])[] | select($card_trace_counts[.rel] != null and $card_trace_counts[.rel] != .card_pack_calls)]) as $card_disagreements
| ($recs | group_by(.arm) | map({key: .[0].arm, value: .}) | from_entries) as $by
| ($known - $present) as $absent
| ([$recs | sort_by([(.arm | armkey), .proj, .start, .agent])[] | select(.check == "disagrees")]) as $disagreements
| ([$recs[] | select(.check == "unrecognized") | .trace] | unique) as $unrecognized
| ["correct", "correct-decl", "stale", "wrong", "nofile", "notree", "unchecked"] as $classes
| (tsv_rows($pack_rows) | group_by(.[0]) | map({key: .[0][0], value: .}) | from_entries) as $span_rows
| (tsv_rows($missed_rows) | group_by(.[0]) | map({key: .[0][0], value: .}) | from_entries) as $missed_by_fork
| (tsv_rows($plan_runs)) as $plan_run_rows
| ([$plan_run_rows[] | select(.[1] != "-" and .[2] != "-") | .[0]]) as $counted_runs
| (tsv_rows($plan_cards)) as $plan_card_rows
| (tsv_rows($plan_refs)) as $plan_ref_rows
| (tsv_rows($plan_writers) | map({key: .[0], value: .}) | from_entries) as $writer_by_run
| def class_counts($rows): [$classes[] as $c | ($rows | map(select(.[7] == $c)) | length)];
  # The counted runs having at least one fork in arm $a; a run whose forks span several arms counts under each.
  def arm_runs($a): [$counted_runs[] | select(. as $r | ($by[$a] | map(.proj) | index($r)) != null)];
  # The reference counts of kind $kind per card of run $r.
  def card_counts($r; $kind):
    [$plan_card_rows[] | select(.[0] == $r) | .[2] as $c | ([$plan_ref_rows[] | select(.[0] == $r and .[2] == $c and .[4] == $kind)] | length)];
  def row($label; c): [$label] + [$present[] | . as $a | $by[$a] | c];
  [
    ["exploration calls, raw", "ex_tools"], ["exploration calls, net", "ex_net"],
    ["whole-file reads, raw", "rfull"], ["whole-file reads, net", "rfull_net"],
    ["partial reads", "rpart"], ["greps", "grep"], ["member greps", "gmem"], ["quarry lookups", "quarry"],
    ["calls before the first edit, net", "pre_edit_net"],
    ["turns", "turns"], ["output tokens", "out"], ["fresh input tokens", "fresh_in"], ["cache reads", "cache_read"]
  ] as $comparison
| (
  ["## Samples", ""]
  + table(["arm", "forks", "runs", "cards", "cross-checked"];
      [$present[] | . as $a | [$a, ($by[$a] | length), ($by[$a] | map(.proj) | unique | length), ($by[$a] | map(.cards) | add),
                               ($by[$a] | map(select(.check != null)) | length)]])
  + (if ($absent | length) > 0 then ["", "Arms with no forks: " + ($absent | join(", ")) + "."] else [] end)
  + (if ($disagreements | length) > 0
     then ["", "Cross-check disagreements:", ""] + [$disagreements[] | "- \(.agent) (\(.proj)): arm \(.arm), trace \(.trace)"]
     else [] end)
  + (if ($card_disagreements | length) > 0
     then ["", "Card pack disagreements:", ""] + [$card_disagreements[] | "- \(.agent) (\(.proj)): card-pack calls \(.card_pack_calls), traced \($card_trace_counts[.rel])"]
     else [] end)
  + (if ($unrecognized | length) > 0 then ["", "Unrecognized trace values: " + ($unrecognized | join(", ")) + "."] else [] end)
  + ["", "## Pack use", ""]
  + table(["metric"] + $present; [
      row("pack file opened"; count_cell(.; packed(.pack_opened))),
      row("pack read in assistant turn, median"; med_cell(.; packed(.pack_turn); n2)),
      row("pack read before the first edit"; count_cell(.; packed(edited(.pack_before_edit)))),
      row("distinct spans per fork, median"; med_cell(.; packed(.nspans); n2)),
      row("distinct spans per card, median"; med_cell(.; packed(per_card("nspans")); n2)),
      row("spans reached before the first edit, strict share, median"; med_cell(.; packed(edited(spanned(.spans_strict_pre / .nspans))); pct)),
      row("spans reached before the first edit, strict share, pooled"; ratio_cell(.; packed(edited(spanned([.spans_strict_pre, .nspans]))); pct)),
      row("span files touched before the first edit, file-level share, median"; med_cell(.; packed(edited(spanned(.spans_file_pre / .nspans))); pct)),
      row("span files touched before the first edit, file-level share, pooled"; ratio_cell(.; packed(edited(spanned([.spans_file_pre, .nspans]))); pct)),
      row("spans reached in the first work turn, strict share, pooled"; ratio_cell(.; packed(spanned(first_turn([.spans_strict_first, .nspans]))); pct)),
      row("spans reached in the first work turn, file-level share, pooled"; ratio_cell(.; packed(spanned(first_turn([.spans_file_first, .nspans]))); pct)),
      row("forks reaching half their spans (strict) before the first edit"; count_cell(.; packed(edited(spanned(.spans_strict_pre / .nspans >= 0.5)))))
    ])
  + (if $card_present then ["", "Card arm: a span counts as reached when read after the card-pack call that lists it and before the next edit; the other arms count a span read before the batch's first edit, and the card arm has no first-work-turn share because that turn reads card files only."] else [] end)
  + ["", "## Arm comparison", ""]
  + table(["metric"] + $present;
      [ $comparison[] as [$label, $key]
        | row($label + ", median per card"; med_cell(.; per_card($key); n2)),
          row($label + ", pooled per card"; ratio_cell(.; pooled_card($key); n2)) ]
      + [row("forks with 3+ parallel calls in the first work turn, share"; (map(select(.first_par >= 3)) | length) / length | pct)])
  + (if $card_present then ["", "Card arm: the first work turn reads card files only, so its share of forks with 3+ parallel calls counts card-file reads, not span reads; the other arms' first work turn holds their span reads."] else [] end)
  + ["", "## Exploration quartiles", ""]
  + table(["arm", "n", "min", "q1", "median", "q3", "max"];
      [ $present[] | . as $a
        | [$by[$a][] | select(.cards > 0) | .ex_net / .cards] as $v
        | if ($v | length) == 0 then [$a, 0, "n/a", "n/a", "n/a", "n/a", "n/a"]
          else [$a, ($v | length), ($v | min | n2), ($v | q(0.25) | n2), ($v | q(0.5) | n2), ($v | q(0.75) | n2), ($v | max | n2)] end ])
  + ["", "## Per-fork", ""]
  + table(
      ["arm", "run", "agent", "start", "cards", "pack opened", "pack turn", "pack before edit", "first work turn", "first parallel", "spans",
       "strict before edit", "file-level before edit", "strict in first turn", "calls before edit", "net before edit", "net exploration",
       "net whole reads", "partial reads", "greps", "quarry", "turns", "output k", "cache read M", "edits", "edits to no-span files", "dropped lines",
       "batch", "trace", "cross-check", "card packs"];
      [$recs | sort_by([(.arm | armkey), .proj, .start, .agent])[]
       | [.arm, .proj, .agent, .start[5:16], .cards, (if .pack_opened then "yes" else "no" end), .pack_turn,
          (if .pack_before_edit then "yes" else "no" end), .first_work_turn, .first_par, .nspans,
          .spans_strict_pre, .spans_file_pre, .spans_strict_first, .pre_edit, .pre_edit_net, .ex_net,
          .rfull_net, .rpart, .grep, .quarry, .turns, (.out / 1000 | round), (.cache_read / 1e6 * 10 | round / 10), .edits, .edits_out, .dropped,
          .batch, .trace, .check,
          ($card_trace_counts[.rel] as $traced
           | if $traced == null then (if .card_pack_calls > 0 then "\(.card_pack_calls)/-" else null end)
             elif .card_pack_calls == 0 and $traced == 0 then null
             else "\(.card_pack_calls)/\($traced)" end)]])
  + (if $card_present then ["", "Card arm: strict before edit and file-level before edit use the window stated under the Pack use table, strict in first turn is - and first parallel counts the card-file reads."] else [] end)
  + (if $pack_check then
      ["", "## Pack correctness at batch start", ""]
      + (if $card_present then ["Card arm: its spans here come from its first card-pack call only, resolved at the batch's start; spans from later calls are resolved against edited trees and are not graded, so the count is below its distinct spans under Pack use.", ""] else [] end)
      + ["### Span entries by class, pooled per arm", ""]
      + table(["arm", "forks", "spans"] + $classes;
          [$present[] | . as $a | [$by[$a][] | $span_rows[.rel] // [] | .[]] as $rows | [$a, ($by[$a] | length), ($rows | length)] + class_counts($rows)])
      + ["", "### Span entries by class, per fork", ""]
      + table(["arm", "run", "agent"] + $classes + ["stale shifts"];
          [$recs | sort_by([(.arm | armkey), .proj, .start, .agent])[]
           | ($span_rows[.rel] // []) as $rows
           | [.arm, .proj, .agent] + class_counts($rows)
             + [($rows | map(select(.[7] == "stale") | .[8]) | if length == 0 then null else join(", ") end)]])
      + ["", "### Missed files by arm", "", "Files a fork edited, wrote or read that its pack gave no span for, split by whether they existed at the batch's start.", ""]
      + table(["arm", "missed files", "existed", "absent", "unknown"];
          [$present[] | . as $a | [$by[$a][] | $missed_by_fork[.rel] // [] | .[]] as $rows
           | [$a, ($rows | length)] + [("yes", "no", "unknown") as $e | ($rows | map(select(.[6] == $e)) | length)]])
      + ["", "### Missed-file list", ""]
      + table(["agent", "file", "edits", "writes", "reads", "existed"];
          [$recs | sort_by([.agent])[] | . as $r | ($missed_by_fork[$r.rel] // []) | sort_by(.[2])[] | [$r.agent, .[2], .[3], .[4], .[5], .[6]]])
    else [] end)
  + (if $plan_on then
      ([$present[] | select(arm_runs(.) | length > 0)]) as $plan_arms
      | ["", "## Plan glyph use", "", "### Reference kinds per card, median", ""]
        + table(["kind"] + $plan_arms;
            [("glyph", "planhandle", "bare_file", "bare_dir") as $kind
             | [({"glyph": "glyph references", "planhandle": "plan: handles", "bare_file": "bare files", "bare_dir": "bare directories"}[$kind])]
               + [$plan_arms[] | . as $a | [arm_runs($a)[] | card_counts(.; $kind)[]] | if length == 0 then null else med end | n2]])
        + ["", "### Plan writers, per arm", ""]
        + table(["metric"] + $plan_arms;
            [(["plan writers", null], ["quarry calls", 3], ["greps", 4], ["reads", 5]) as [$label, $column]
             | if $column == null
               then [$label] + [$plan_arms[] | arm_runs(.) | length]
               else ([$label + ", pooled"] + [$plan_arms[] | . as $a | [arm_runs($a)[] | $writer_by_run[.][$column] | tonumber] | add])
                    , ([$label + ", median per writer"] + [$plan_arms[] | . as $a | [arm_runs($a)[] | $writer_by_run[.][$column] | tonumber] | med | n2])
               end])
        + (if ($plan_run_rows | map(select(.[1] == "-" or .[2] == "-")) | length) > 0
           then ["", "### Runs not counted", ""]
                + [$plan_run_rows[] | select(.[1] == "-") | "- run \(.[0]): no records commit found"]
                + [$plan_run_rows[] | select(.[1] != "-" and .[2] == "-") | "- run \(.[0]): no plan-writer session found"]
           else [] end)
    else [] end)
) | .[]
