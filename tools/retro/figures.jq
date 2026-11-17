# figures.jq renders the retro figures as markdown from the files retro.sh writes,
# each passed slurped under its own name: $sessions, $refusals, $rows, $facts, $keys, $missing and $usage.
# A producer's outlier is a session of a row with at least three sessions,
# whose turns or cost reach twice the row's median.

def median: sort | if length == 0 then 0 elif length % 2 == 1 then .[length / 2 | floor] else (.[length / 2 - 1] + .[length / 2]) / 2 end;
def money: . * 100 | round / 100 | tostring | "$" + .;
def cell: tostring | gsub("\\|"; "\\|");

($sessions | map(select(.kind == "main"))) as $main
| ([$sessions[].slug, $rows[].slug, $usage[].slug] | unique) as $slugs
| "# Retro figures",
  "",
  "## Runs",
  "",
  "| Run | Sessions | Forks | Cost | Verify gate runs | Verify reworks | Gate-fixer sessions | Review rounds |",
  "| --- | --- | --- | --- | --- | --- | --- | --- |",
  ($slugs[] as $s
   | ([$facts[] | select(.slug == $s)] | group_by(.perch) | map("\(.[0].perch) \(map(.round) | max)") | join(", ")) as $rounds
   | "| \($s) | \([$main[] | select(.slug == $s)] | length) | \([$sessions[] | select(.slug == $s and .kind == "fork")] | length)"
     + " | \([$usage[] | select(.slug == $s) | .cost] | add // 0 | money)"
     + " | \([$rows[] | select(.slug == $s) | .verify_runs] | add // 0) | \([$rows[] | select(.slug == $s) | .verify_reworks] | add // 0)"
     + " | \([$main[] | select(.slug == $s and (.title | endswith(":gate-fixer")))] | length) | \(if $rounds == "" then "none" else $rounds end) |"),
  "",
  "## Producers against their medians",
  "",
  "| Row | Sessions | Median turns | Median cost | Outliers |",
  "| --- | --- | --- | --- | --- |",
  ($main | map(select(.row != "")) | group_by(.row)[]
   | (map(.turns) | median) as $mt | (map(.cost) | median) as $mc
   | (if length >= 3 then [.[] | select(.turns >= 2 * $mt or .cost >= 2 * $mc) | "\(.slug) \(.session[0:8]) (\(.turns) turns, \(.cost | money))"] else [] end) as $out
   | "| \(.[0].row) | \(length) | \($mt) | \($mc | money) | \(if ($out | length) == 0 then "-" else $out | join("; ") end) |"),
  "",
  "## Serial single lookups",
  "",
  "Turns that made one read or search right after a turn that did the same, per session with any.",
  "",
  "| Run | Session | Kind | Title | Turns | Lookup turns | Serial |",
  "| --- | --- | --- | --- | --- | --- | --- |",
  ($sessions | map(select(.serial_saved > 0)) | sort_by(-.serial_saved)[]
   | "| \(.slug) | \(.session[0:8]) | \(.kind) | \(.title | cell) | \(.turns) | \(.lookup_turns) | \(.serial_saved) |"),
  "",
  "## Refused commands",
  "",
  "| Refusal | Count |",
  "| --- | --- |",
  ($refusals | group_by(.refusal[0:60]) | sort_by(-length)[] | "| \(.[0].refusal[0:60] | cell) | \(length) |"),
  "",
  "| Run | Session | Kind | Turn | Tool | Refusal |",
  "| --- | --- | --- | --- | --- | --- |",
  ($refusals[] | "| \(.slug) | \(.session[0:8]) | \(.kind) | \(.turn) | \(.tool) | \(.refusal | cell) |"),
  "",
  "## Agent gate runs",
  "",
  "| Run | Session | Title | Gate runs | Failed |",
  "| --- | --- | --- | --- | --- |",
  ($sessions | map(select(.gate_runs > 0))[] | "| \(.slug) | \(.session[0:8]) | \(.title | cell) | \(.gate_runs) | \(.gate_fails) |"),
  "",
  "## Review findings by column",
  "",
  "Each review perch's rounds summed over the runs; a column is a severity or a finding kind of the facts table.",
  "",
  "| Perch | Column | Total |",
  "| --- | --- | --- |",
  ($facts | map(select(.value | test("^[0-9]+$"))) | group_by([.perch, .column])[]
   | "| \(.[0].perch) | \(.[0].column) | \(map(.value | tonumber) | add) |"),
  "",
  "## Ledger keys open in more than one round",
  "",
  ($keys[] | "- \(.slug) \(.perch): `\(.key)`, rounds \(.rounds)"),
  "",
  "## Missing records",
  "",
  ($missing[] | "- \(.slug): no \(.what)")
