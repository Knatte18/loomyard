# report.jq renders the report body from the slurped per-fork records, one markdown line per output string.
# Arguments: $arms, the marker table's arm names in table order.
# Rows and arms sort by arm (marker-table order, then any other arm name alphabetically), run, start time and agent id,
# so two runs over the same input differ only in the generation-time line.

def armkey: [($arms | index(.)) // 1000, .];
def dash: if . == null then "-" else tostring end;
def table($head; $rows):
  ["| " + ($head | join(" | ")) + " |", "| " + ($head | map("---") | join(" | ")) + " |"]
  + [$rows[] | "| " + (map(dash) | join(" | ")) + " |"];

. as $recs
| ($arms + ["unknown"]) as $known
| ([$recs[].arm] | unique | sort_by(armkey)) as $present
| ($recs | group_by(.arm) | map({key: .[0].arm, value: .}) | from_entries) as $by
| ($known - $present) as $absent
| (
  ["## Samples", ""]
  + table(["arm", "forks", "runs", "cards"];
      [$present[] | . as $a | [$a, ($by[$a] | length), ($by[$a] | map(.proj) | unique | length), ($by[$a] | map(.cards) | add)]])
  + (if ($absent | length) > 0 then ["", "Arms with no forks: " + ($absent | join(", ")) + "."] else [] end)
  + ["", "## Per-fork", ""]
  + table(
      ["arm", "run", "agent", "start", "cards", "pack opened", "pack turn", "pack before edit", "first work turn", "first parallel", "spans",
       "strict before edit", "file-level before edit", "strict in first turn", "calls before edit", "net before edit", "net exploration",
       "net whole reads", "partial reads", "greps", "quarry", "turns", "output k", "cache read M", "edits", "edits to no-span files", "dropped lines"];
      [$recs | sort_by([(.arm | armkey), .proj, .start, .agent])[]
       | [.arm, .proj, .agent, .start[5:16], .cards, (if .pack_opened then "yes" else "no" end), .pack_turn,
          (if .pack_before_edit then "yes" else "no" end), .first_work_turn, .first_par, .nspans,
          .spans_strict_pre, .spans_file_pre, .spans_strict_first, .pre_edit, .pre_edit_net, .ex_net,
          .rfull_net, .rpart, .grep, .quarry, .turns, (.out / 1000 | round), (.cache_read / 1e6 * 10 | round / 10), .edits, .edits_out, .dropped]])
) | .[]
